"""
Document Conversion Service — docling ingest + CPU layout + pluggable LLM backend.

Config lives in JSON (config.json, mounted at /config/config.json), NOT chart
values. Secrets are never in config: the API key is read from the env var named
in backend.<mode>.api_key_env (injected from a k8s Secret).

Pipeline:
  ingest (docling) → classify (text vs scanned) → recognize (LLM backend) → format

- Layout detection runs on CPU in-pod (PP-DocLayoutV3), its own /v1/layout endpoint.
- Recognition runs on the LLM backend over OpenAI /v1:
    * backend.mode=external → your Studio/Ollama (GPU queue)
    * backend.mode=local    → in-pod llama-server sidecar (localhost, GPU)
- Recognizer is per-request selectable: dots (end-to-end) | paddle-vl (element crops).
- Field extraction (format=json) routes to the instruction-following extractor.

Endpoints:
  GET  /health
  POST /v1/layout          → CPU layout boxes  {boxes:[{bbox,category,score}]}
  POST /v1/convert         → md|html|text|json|layout-json|csv|tables
  POST /v1/convert/source  → back-compat (Studio docling client), returns md_content
  POST /render             → page images as base64 PNG (back-compat)
"""
import os, io, re, csv, json, base64, time, logging, statistics
from collections import OrderedDict
from fastapi import FastAPI, Request
from fastapi.responses import JSONResponse

import httpx
import pypdfium2 as pdfium
from PIL import Image
from docling.datamodel.base_models import InputFormat, DocumentStream
from docling.datamodel.pipeline_options import PdfPipelineOptions
from docling.document_converter import DocumentConverter, PdfFormatOption

log = logging.getLogger("conv")
logging.basicConfig(level=logging.INFO)

IMAGE_EXTS = {"png", "jpg", "jpeg", "tif", "tiff", "bmp", "webp", "gif"}
_HERE = os.path.dirname(os.path.abspath(__file__))


# ---------------------------------------------------------------- config
def load_config():
    for p in (os.environ.get("CONFIG_PATH", "/config/config.json"),
              os.path.join(_HERE, "config.json")):
        try:
            with open(p) as f:
                log.info("loaded config from %s", p)
                return json.load(f)
        except Exception:
            continue
    log.warning("no config.json found; using minimal defaults")
    return {}

CFG = load_config()
DEF = CFG.get("defaults", {})
PROMPTS = CFG.get("prompts", {})
TEXT_THRESHOLD = int(DEF.get("text_threshold", 40))
SCALE = float(DEF.get("render_scale", 2.0))
MAX_PAGES = int(DEF.get("max_vlm_pages", 10))


def backend():
    b = CFG.get("backend", {})
    mode = os.environ.get("BACKEND_MODE", b.get("mode", "external"))
    conf = b.get(mode) or b.get("external", {})
    # env overrides let Helm inject the namespaced URL / key without editing JSON
    url = (os.environ.get("LLM_BASE_URL") or conf.get("base_url", "")).rstrip("/")
    key = os.environ.get(conf.get("api_key_env", "STUDIO_API_KEY"), "")
    return url, key, int(conf.get("timeout", 300))


def recognizer_cfg(name):
    r = CFG.get("recognizers", {})
    return r.get(name) or r.get(DEF.get("recognizer", "dots")) or {}


# ---------------------------------------------------------------- LLM backend (OpenAI /v1)
def llm_chat(model, prompt, image_png=None, options=None):
    url, key, timeout = backend()
    content = [{"type": "text", "text": prompt}]
    if image_png is not None:
        b64 = base64.b64encode(image_png).decode()
        content.append({"type": "image_url",
                        "image_url": {"url": f"data:image/png;base64,{b64}"}})
    opts = dict(options or {})
    payload = {"model": model, "stream": False,
               "messages": [{"role": "user", "content": content}],
               "temperature": opts.get("temperature", 0)}
    if "num_predict" in opts:
        payload["max_tokens"] = opts["num_predict"]
    headers = {"Content-Type": "application/json"}
    if key:
        headers["Authorization"] = f"Bearer {key}"
    r = httpx.post(f"{url}/chat/completions", json=payload, headers=headers, timeout=timeout)
    r.raise_for_status()
    return (r.json()["choices"][0]["message"]["content"] or "")


def _strip_fence(s):
    s = (s or "").strip()
    if s.startswith("```"):
        parts = s.split("```")
        if len(parts) >= 3:
            s = parts[1]
            if "\n" in s:
                s = s.split("\n", 1)[1]
        s = s.strip()
    return s


# ---------------------------------------------------------------- docling CPU (text PDFs / Office)
_cpu = None
def cpu_converter():
    global _cpu
    if _cpu is None:
        popt = PdfPipelineOptions(do_ocr=False)
        _cpu = DocumentConverter(format_options={
            InputFormat.PDF: PdfFormatOption(pipeline_options=popt)})
    return _cpu


def pdf_has_text(data):
    try:
        pdf = pdfium.PdfDocument(data)
        n = min(len(pdf), 5)
        total = sum(len(pdf[i].get_textpage().get_text_range().strip()) for i in range(n))
        return (total / max(1, n)) >= TEXT_THRESHOLD
    except Exception as e:
        log.warning("pdf text probe failed (%s) → scanned", e)
        return False


# ---------------------------------------------------------------- rendering
def page_pngs(data, scale=None, limit=None):
    scale = scale or SCALE
    pdf = pdfium.PdfDocument(data)
    n = len(pdf)
    out = []
    for i in range(n if limit is None else min(n, limit)):
        pil = pdf[i].render(scale=scale).to_pil().convert("RGB")
        buf = io.BytesIO(); pil.save(buf, "PNG")
        out.append(buf.getvalue())
    return out, n


def image_png(data):
    pil = Image.open(io.BytesIO(data)).convert("RGB")
    buf = io.BytesIO(); pil.save(buf, "PNG")
    return buf.getvalue()


# ---------------------------------------------------------------- CPU layout (PP-DocLayoutV3)
_layout = None
def layout_model():
    global _layout
    if _layout is None:
        os.environ.setdefault("PADDLE_PDX_MODEL_SOURCE", "huggingface")
        from paddleocr import LayoutDetection
        _layout = LayoutDetection(model_name=CFG.get("layout", {}).get("model_name", "PP-DocLayoutV3"))
    return _layout


def detect_layout(png):
    import numpy as np
    th = float(CFG.get("layout", {}).get("threshold", 0.3))
    pil = Image.open(io.BytesIO(png)).convert("RGB")
    res = list(layout_model().predict(np.array(pil), batch_size=1, threshold=th))
    boxes = res[0]["boxes"] if res else []
    out = []
    for b in boxes:
        c = b.get("coordinate") or b.get("bbox")
        out.append({"bbox": [int(v) for v in c], "category": b.get("label", ""),
                    "score": round(float(b.get("score", 0)), 3)})
    out.sort(key=lambda e: (round(e["bbox"][1] / 40), e["bbox"][0]))
    return out


# ---------------------------------------------------------------- OTSL helpers (PaddleOCR-VL tables)
def otsl_to_rows(otsl):
    rows = []
    for row in otsl.split("<nl>"):
        if not row.strip():
            continue
        cells = [m.group(1).strip() if m.group(0).startswith("<fcel>") else ""
                 for m in re.finditer(r"<fcel>([^<]*)|<ecel>", row)]
        if cells:
            rows.append(cells)
    return rows


def rows_to_html(rows):
    return "<table>" + "".join(
        "<tr>" + "".join(f"<td>{c}</td>" for c in r) + "</tr>" for r in rows) + "</table>"


def rows_to_csv(rows):
    buf = io.StringIO(); w = csv.writer(buf)
    for r in rows:
        w.writerow(r)
    return buf.getvalue()


# ---------------------------------------------------------------- text-PDF tables → CSV (docling geometry, no LLM)
def _center(r):
    xs = [r.r_x0, r.r_x1, r.r_x2, r.r_x3]; ys = [r.r_y0, r.r_y1, r.r_y2, r.r_y3]
    return (min(xs) + max(xs)) / 2, (min(ys) + max(ys)) / 2, (max(ys) - min(ys))


def _cell_lines(tcell, pcs, tol):
    """Rebuild a cell's physical lines from page word centers (docling flattens
    wrapped lines to spaces; geometry recovers them)."""
    bb = tcell.bbox
    inside = [(cy, cx, c) for cx, cy, c in pcs
              if bb.l - 0.5 <= cx <= bb.r + 0.5 and bb.t - 0.5 <= cy <= bb.b + 0.5]
    inside.sort()
    lines, prev = [], None
    for cy, cx, txt in inside:
        if prev is None or cy - prev > tol:
            lines.append([])
        lines[-1].append((cx, txt)); prev = cy
    out = [" ".join(t for _, t in sorted(l)).strip() for l in lines]
    if out:
        return out
    return [(tcell.text or "").strip()] if (tcell.text or "").strip() else [""]


def text_pdf_tables(data, filename):
    """Deterministic table extraction for text PDFs/Office: merge same-header
    tables (multi-page) + explode multi-line cells. Returns list[rows]."""
    res = cpu_converter().convert(source=DocumentStream(name=filename, stream=io.BytesIO(data)))
    doc = res.document
    groups = OrderedDict()
    for t in doc.tables:
        td = t.data
        nr, nc = td.num_rows, td.num_cols
        page = res.pages[t.prov[0].page_no - 1] if t.prov else res.pages[0]
        pcs, heights = [], []
        for c in page.cells:
            if (c.text or "").strip():
                cx, cy, h = _center(c.rect)
                pcs.append((cx, cy, c.text)); heights.append(h)
        tol = 0.6 * statistics.median(heights) if heights else 4.0
        grid = [[[""] for _ in range(nc)] for _ in range(nr)]
        for tc in td.table_cells:
            r, cc = tc.start_row_offset_idx, tc.start_col_offset_idx
            if 0 <= r < nr and 0 <= cc < nc:
                grid[r][cc] = _cell_lines(tc, pcs, tol)
        header = tuple(" ".join(cell).strip() for cell in grid[0]) if nr else ()
        groups.setdefault(header, []).append(grid[1:])
    tables = []
    for header, row_lists in groups.items():
        rows = [list(header)]
        for rowset in row_lists:
            for row in rowset:
                depth = max((len(cell) for cell in row), default=1)
                for k in range(depth):
                    rows.append([(cell[k] if k < len(cell) else "") for cell in row])
        tables.append(rows)
    return tables


# ---------------------------------------------------------------- recognition
def recognize_page_endtoend(png, model, opts, prompt):
    """dots-style: whole page → markdown/html in one call."""
    return _strip_fence(llm_chat(model, prompt, png, opts))


def recognize_page_element(png, model, opts, want):
    """paddle-vl style: CPU layout → per-region crop → OCR/table → assemble.
    want in {'markdown','html','csv','tables'}."""
    pil = Image.open(io.BytesIO(png)).convert("RGB")
    regions = detect_layout(png)
    parts, tables = [], []
    for reg in regions:
        cat = reg["category"].lower()
        x0, y0, x1, y1 = reg["bbox"]
        crop = pil.crop((x0, y0, x1, y1))
        buf = io.BytesIO(); crop.save(buf, "PNG"); cpng = buf.getvalue()
        if cat in ("image", "picture"):
            continue
        if "table" in cat:
            otsl = llm_chat(model, PROMPTS.get("table", "Table Recognition:"), cpng, opts)
            rows = otsl_to_rows(otsl) if "<fcel>" in otsl else []
            tables.append(rows)
            if want in ("markdown", "html"):
                parts.append(rows_to_html(rows) if rows else "<pre>" + otsl + "</pre>")
        else:
            txt = llm_chat(model, PROMPTS.get("ocr_text", "OCR:"), cpng, opts).strip()
            if want in ("markdown", "html"):
                parts.append(("<p>" + txt.replace("\n", "<br>") + "</p>") if want == "html" else txt)
    if want in ("csv", "tables"):
        return {"tables": tables}
    return {"content": "\n\n".join(p for p in parts if p)}


# ---------------------------------------------------------------- format dispatch
def convert_one(filename, data, fmt, recognizer, schema, options):
    ext = filename.rsplit(".", 1)[-1].lower() if "." in filename else ""
    is_image = ext in IMAGE_EXTS
    is_pdf = ext == "pdf"
    text_pdf = is_pdf and pdf_has_text(data)
    rc = recognizer_cfg(recognizer)
    model = rc.get("model")
    opts = {**rc.get("options", {}), **(options or {})}

    # ---- text PDFs / Office / CSV → docling CPU (no LLM) for text formats
    if not is_image and not (is_pdf and not text_pdf):
        res = cpu_converter().convert(source=DocumentStream(name=filename, stream=io.BytesIO(data)))
        doc = res.document
        if fmt == "markdown":
            return {"content": doc.export_to_markdown(), "pipeline": "cpu"}
        if fmt == "html":
            return {"content": doc.export_to_html(), "pipeline": "cpu"}
        if fmt == "text":
            return {"content": doc.export_to_text(), "pipeline": "cpu"}
        if fmt in ("csv", "tables"):
            tabs = text_pdf_tables(data, filename)
            if fmt == "tables":
                return {"content": [rows_to_html(t) for t in tabs], "pipeline": "cpu-geometry"}
            return {"content": "\n\n".join(rows_to_csv(t) for t in tabs), "pipeline": "cpu-geometry"}
        # json / layout-json from a text doc fall through to the model paths (render below)

    # ---- pages to feed the model
    if is_image:
        pngs = [image_png(data)]
    elif is_pdf:
        pngs, _ = page_pngs(data, limit=MAX_PAGES)
    else:
        # office → render via docling to text then wrap (rare for these formats)
        res = cpu_converter().convert(source=DocumentStream(name=filename, stream=io.BytesIO(data)))
        return {"content": res.document.export_to_markdown(), "pipeline": "cpu"}

    # ---- JSON field extraction → extractor model
    if fmt == "json":
        ex = CFG.get("extractor", {})
        prompt = PROMPTS.get("extract_json", "Extract as JSON. Output only JSON.")
        if schema:
            prompt += "\nJSON schema:\n" + json.dumps(schema)
        raw = llm_chat(ex.get("model"), prompt, pngs[0], ex.get("options", {}))
        raw = _strip_fence(raw)
        a, b = raw.find("{"), raw.rfind("}")
        cand = raw[a:b + 1]
        cand = re.sub(r",\s*([}\]])", r"\1", cand)          # tolerate trailing commas
        cand = re.sub(r':\s*(-?\d+),(\d+)\s*([,}\]])', r': "\1,\2"\3', cand)  # quote comma-decimals
        try:
            return {"content": json.loads(cand), "pipeline": "extract"}
        except Exception:
            return {"content": raw, "pipeline": "extract", "warning": "unparseable json"}

    # ---- layout-json → dots native layout mode (end-to-end)
    if fmt == "layout-json":
        raw = _strip_fence(llm_chat(model, PROMPTS.get("layout_json", ""), pngs[0], opts))
        try:
            return {"content": json.loads(raw if raw.startswith("[") else "[" + raw + "]"),
                    "pipeline": "layout-json"}
        except Exception:
            return {"content": raw, "pipeline": "layout-json", "warning": "unparseable"}

    # ---- csv / tables → element pipeline (layout crops + table recognition)
    if fmt in ("csv", "tables"):
        all_tables = []
        for png in pngs:
            r = recognize_page_element(png, model, opts, "tables")
            all_tables += r["tables"]
        if fmt == "tables":
            return {"content": [rows_to_html(t) for t in all_tables], "pipeline": "element"}
        return {"content": "\n\n".join(rows_to_csv(t) for t in all_tables), "pipeline": "element"}

    # ---- markdown / html / text (transcription)
    prompt = PROMPTS.get("transcribe_html" if fmt == "html" else "transcribe_md",
                         PROMPTS.get("transcribe_page", "Transcribe to Markdown."))
    mode = rc.get("mode", "end-to-end")
    outs = []
    for png in pngs:
        if mode == "element":
            outs.append(recognize_page_element(png, model, opts, fmt if fmt != "text" else "markdown")["content"])
        else:
            outs.append(recognize_page_endtoend(png, model, opts, prompt))
    content = "\n\n".join(o for o in outs if o)
    if fmt == "text":
        content = re.sub(r"<[^>]+>", " ", content)
    return {"content": content, "pipeline": mode}


# ---------------------------------------------------------------- app
app = FastAPI()


@app.get("/health")
def health():
    url, _, _ = backend()
    return {"status": "ok", "backend": url, "formats": CFG.get("formats", [])}


@app.post("/v1/layout")
async def v1_layout(req: Request):
    body = await req.json()
    try:
        data = base64.b64decode(body.get("base64_string", ""))
    except Exception:
        return JSONResponse({"error": "invalid base64_string"}, status_code=400)
    fn = body.get("filename", "document")
    ext = fn.rsplit(".", 1)[-1].lower() if "." in fn else ""
    png = image_png(data) if ext in IMAGE_EXTS else page_pngs(data, limit=1)[0][0]
    t0 = time.time()
    try:
        boxes = detect_layout(png)
    except Exception as e:
        log.exception("layout failed")
        return JSONResponse({"error": f"layout failed: {e}"}, status_code=502)
    return {"boxes": boxes, "_ms": int((time.time() - t0) * 1000)}


@app.post("/v1/convert")
async def v1_convert(req: Request):
    body = await req.json()
    srcs = body.get("sources") or []
    if not srcs:
        return JSONResponse({"error": "no sources provided"}, status_code=400)
    src = srcs[0]
    fn = src.get("filename", "document")
    try:
        data = base64.b64decode(src.get("base64_string", ""))
    except Exception:
        return JSONResponse({"error": "invalid base64_string"}, status_code=400)
    fmt = body.get("format", DEF.get("format", "markdown"))
    recognizer = body.get("recognizer", DEF.get("recognizer", "dots"))
    schema = body.get("schema")
    options = body.get("options") or {}
    if fmt not in CFG.get("formats", ["markdown"]):
        return JSONResponse({"error": f"unsupported format {fmt}"}, status_code=400)
    t0 = time.time()
    try:
        r = convert_one(fn, data, fmt, recognizer, schema, options)
    except httpx.HTTPStatusError as e:
        return JSONResponse({"error": f"llm upstream {e.response.status_code}: {e.response.text[:200]}"},
                            status_code=502)
    except Exception as e:
        log.exception("convert failed for %s", fn)
        return JSONResponse({"error": f"convert failed: {e}"}, status_code=502)
    return {"document": {"filename": fn, "format": fmt, "content": r.get("content")},
            "status": "success", "_recognizer": recognizer,
            "_pipeline": r.get("pipeline"), "_ms": int((time.time() - t0) * 1000),
            **({"warning": r["warning"]} if r.get("warning") else {})}


@app.post("/v1/convert/source")
async def convert_source(req: Request):
    """Back-compat: Studio docling client. Returns md_content."""
    body = await req.json()
    srcs = body.get("sources") or []
    if not srcs:
        return JSONResponse({"error": "no sources provided"}, status_code=400)
    src = srcs[0]
    fn = src.get("filename", "document")
    try:
        data = base64.b64decode(src.get("base64_string", ""))
    except Exception:
        return JSONResponse({"error": "invalid base64_string"}, status_code=400)
    t0 = time.time()
    try:
        r = convert_one(fn, data, "markdown", DEF.get("recognizer", "dots"), None, {})
    except Exception as e:
        log.exception("convert/source failed for %s", fn)
        return JSONResponse({"error": f"convert failed: {e}"}, status_code=502)
    return {"document": {"filename": fn, "md_content": r.get("content")},
            "status": "success", "_pipeline": r.get("pipeline"),
            "_ms": int((time.time() - t0) * 1000)}


@app.post("/render")
async def render(req: Request):
    """Back-compat: return page images as base64 PNG for scanned docs."""
    body = await req.json()
    fn = body.get("filename", "document")
    try:
        mx = int(body.get("max_pages", MAX_PAGES))
    except Exception:
        mx = MAX_PAGES
    try:
        data = base64.b64decode(body.get("base64_string", ""))
    except Exception:
        return JSONResponse({"error": "invalid base64_string"}, status_code=400)
    ext = fn.rsplit(".", 1)[-1].lower() if "." in fn else ""
    if ext in IMAGE_EXTS:
        return {"kind": "image", "scanned": True, "page_count": 1,
                "images": [base64.b64encode(image_png(data)).decode()]}
    if ext == "pdf":
        scanned = not pdf_has_text(data)
        pngs, n = page_pngs(data, limit=mx) if scanned else ([], len(pdfium.PdfDocument(data)))
        imgs = [base64.b64encode(p).decode() for p in pngs] if (scanned and n <= mx) else []
        return {"kind": "pdf", "scanned": scanned, "page_count": n, "images": imgs}
    return {"kind": "other", "scanned": False, "page_count": 0, "images": []}
