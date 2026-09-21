"""Pre-download the CPU-pipeline models (layout + TableFormer) into the model
cache. Run as an init container because our wrapper replaces docling-serve's
entrypoint, which is what normally downloads models at boot. OCR/VLM models are
skipped: scanned docs are handled by the remote vision model, not local OCR.

Idempotent: if the layout weights already exist (cache persists on the hostPath),
it exits immediately, so it only ever downloads once."""
from pathlib import Path

ARTIFACTS = Path("/modelcache/docling")
LAYOUT = ARTIFACTS / "ds4sd--docling-models" / "model_artifacts" / "layout" / "model.safetensors"

if LAYOUT.exists():
    print("docling models already present; skipping download")
    raise SystemExit(0)

from docling.utils.model_downloader import download_models

print("downloading docling CPU models (layout + tableformer)…")
download_models(
    output_dir=ARTIFACTS, progress=False,
    with_layout=True, with_tableformer=True,
    with_code_formula=False, with_picture_classifier=False, with_easyocr=False,
)
print("docling models downloaded to", ARTIFACTS)
