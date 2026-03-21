package files

import (
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// FileInfo is the JSON shape the frontend expects from GET /api/resources.
type FileInfo struct {
	Path      string     `json:"path"`
	Name      string     `json:"name"`
	Size      int64      `json:"size"`
	Extension string     `json:"extension"`
	Modified  time.Time  `json:"modified"`
	Mode      uint32     `json:"mode"`
	IsDir     bool       `json:"isDir"`
	IsSymlink bool       `json:"isSymlink"`
	Type      string     `json:"type"`
	Content   string     `json:"content,omitempty"`
	Items     []FileInfo `json:"items,omitempty"`
	NumDirs   int        `json:"numDirs"`
	NumFiles  int        `json:"numFiles"`
}

// Handler holds the configuration for the file manager API.
type Handler struct {
	DataPath      string
	MaxUploadSize int64
}

// NewHandler creates a new file handler rooted at dataPath.
func NewHandler(dataPath string, maxUploadSize int64) *Handler {
	return &Handler{
		DataPath:      dataPath,
		MaxUploadSize: maxUploadSize,
	}
}

// RegisterRoutes wires up all file manager routes on the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/resources", h.handleResources)
	mux.HandleFunc("/api/resources/", h.handleResources)
	mux.HandleFunc("/api/raw", h.handleRaw)
	mux.HandleFunc("/api/raw/", h.handleRaw)
	mux.HandleFunc("/api/paste", h.handlePaste)
	mux.HandleFunc("/api/paste/", h.handlePaste)
	mux.HandleFunc("/api/preview/", h.handlePreview)
	mux.HandleFunc("/api/nodes/", h.handleNodes)
	mux.HandleFunc("/upload", h.handleUploadInit)
	mux.HandleFunc("/upload/", h.handleUploadChunk)
	mux.HandleFunc("/upload/upload-link/", h.handleUploadLink)
	mux.HandleFunc("/upload/file-uploaded-bytes/", h.handleUploadedBytes)
}

// safePath validates and resolves a request path to a real filesystem path,
// ensuring it stays within DataPath. Returns ("", error) on traversal attempt.
func (h *Handler) safePath(reqPath string) (string, error) {
	// Strip leading /data or /Home prefix variants
	cleaned := reqPath
	for _, prefix := range []string{"/data", "/Home"} {
		if strings.HasPrefix(cleaned, prefix) {
			cleaned = strings.TrimPrefix(cleaned, prefix)
			break
		}
	}
	if cleaned == "" {
		cleaned = "/"
	}

	joined := filepath.Join(h.DataPath, cleaned)
	abs, err := filepath.Abs(joined)
	if err != nil {
		return "", fmt.Errorf("invalid path")
	}

	// Ensure the resolved path is within DataPath
	dataAbs, _ := filepath.Abs(h.DataPath)
	if !strings.HasPrefix(abs, dataAbs) {
		return "", fmt.Errorf("path traversal denied")
	}

	return abs, nil
}

func (h *Handler) handleResources(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listOrStat(w, r)
	case http.MethodPost:
		h.uploadFile(w, r)
	case http.MethodPut:
		h.createOrUpdate(w, r)
	case http.MethodDelete:
		h.deleteResource(w, r)
	case http.MethodPatch:
		h.renameMove(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) listOrStat(w http.ResponseWriter, r *http.Request) {
	reqPath := r.URL.Path
	// Strip the /api/resources prefix
	reqPath = strings.TrimPrefix(reqPath, "/api/resources")
	if reqPath == "" {
		reqPath = "/"
	}

	absPath, err := h.safePath(reqPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "not found", http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	// If it's a file and client wants a stream, serve the file
	if !info.IsDir() && r.URL.Query().Get("stream") == "1" {
		http.ServeFile(w, r, absPath)
		return
	}

	fi := h.buildFileInfo(absPath, reqPath, info)

	// For text files, include content
	if !info.IsDir() {
		ext := strings.ToLower(filepath.Ext(info.Name()))
		if isTextExt(ext) && info.Size() < 1024*1024 {
			data, err := os.ReadFile(absPath)
			if err == nil {
				fi.Content = string(data)
			}
		}
	}

	// For directories, list children
	if info.IsDir() {
		entries, err := os.ReadDir(absPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		fi.Items = make([]FileInfo, 0, len(entries))
		for _, entry := range entries {
			childInfo, err := entry.Info()
			if err != nil {
				continue
			}
			childPath := filepath.Join(reqPath, entry.Name())
			if !strings.HasPrefix(childPath, "/") {
				childPath = "/" + childPath
			}
			child := h.buildFileInfo(filepath.Join(absPath, entry.Name()), childPath, childInfo)
			if entry.IsDir() {
				fi.NumDirs++
			} else {
				fi.NumFiles++
			}
			fi.Items = append(fi.Items, child)
		}

		// Sort: dirs first, then by name
		sort.Slice(fi.Items, func(i, j int) bool {
			if fi.Items[i].IsDir != fi.Items[j].IsDir {
				return fi.Items[i].IsDir
			}
			return strings.ToLower(fi.Items[i].Name) < strings.ToLower(fi.Items[j].Name)
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(fi)
}

func (h *Handler) buildFileInfo(absPath, reqPath string, info os.FileInfo) FileInfo {
	ext := filepath.Ext(info.Name())
	fType := detectFileType(info.Name(), info.IsDir())
	isLink := info.Mode()&os.ModeSymlink != 0

	return FileInfo{
		Path:      reqPath,
		Name:      info.Name(),
		Size:      info.Size(),
		Extension: strings.TrimPrefix(ext, "."),
		Modified:  info.ModTime(),
		Mode:      uint32(info.Mode()),
		IsDir:     info.IsDir(),
		IsSymlink: isLink,
		Type:      fType,
	}
}

func (h *Handler) uploadFile(w http.ResponseWriter, r *http.Request) {
	reqPath := strings.TrimPrefix(r.URL.Path, "/api/resources")
	if reqPath == "" {
		reqPath = "/"
	}

	absPath, err := h.safePath(reqPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	override := r.URL.Query().Get("override") == "true"

	// Check content type
	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/") {
		// Multipart upload
		if err := r.ParseMultipartForm(h.MaxUploadSize); err != nil {
			http.Error(w, "file too large", http.StatusRequestEntityTooLarge)
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			// If no multipart file, treat the body as the file content
			h.uploadRawBody(w, r, absPath, override)
			return
		}
		defer file.Close()

		destPath := filepath.Join(absPath, header.Filename)
		if !override {
			if _, err := os.Stat(destPath); err == nil {
				http.Error(w, "file already exists", http.StatusConflict)
				return
			}
		}

		// Ensure parent directory exists
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		dst, err := os.Create(destPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer dst.Close()

		if _, err := io.Copy(dst, file); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		return
	}

	// Raw body upload
	h.uploadRawBody(w, r, absPath, override)
}

func (h *Handler) uploadRawBody(w http.ResponseWriter, r *http.Request, absPath string, override bool) {
	if !override {
		if _, err := os.Stat(absPath); err == nil {
			http.Error(w, "file already exists", http.StatusConflict)
			return
		}
	}

	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	dst, err := os.Create(absPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, io.LimitReader(r.Body, h.MaxUploadSize)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (h *Handler) createOrUpdate(w http.ResponseWriter, r *http.Request) {
	reqPath := strings.TrimPrefix(r.URL.Path, "/api/resources")
	if reqPath == "" {
		reqPath = "/"
	}

	absPath, err := h.safePath(reqPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	contentType := r.Header.Get("Content-Type")

	if strings.HasSuffix(reqPath, "/") || contentType == "" {
		// Create directory
		if err := os.MkdirAll(absPath, 0755); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		return
	}

	// Update file content (text save)
	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, h.MaxUploadSize))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := os.WriteFile(absPath, body, 0644); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (h *Handler) deleteResource(w http.ResponseWriter, r *http.Request) {
	reqPath := strings.TrimPrefix(r.URL.Path, "/api/resources")
	if reqPath == "" || reqPath == "/" {
		http.Error(w, "cannot delete root", http.StatusForbidden)
		return
	}

	absPath, err := h.safePath(reqPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	if err := os.RemoveAll(absPath); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (h *Handler) renameMove(w http.ResponseWriter, r *http.Request) {
	reqPath := strings.TrimPrefix(r.URL.Path, "/api/resources")
	if reqPath == "" {
		reqPath = "/"
	}

	q := r.URL.Query()
	action := q.Get("action")
	destination := q.Get("destination")
	overrideStr := q.Get("override")
	renameStr := q.Get("rename")

	if destination == "" {
		http.Error(w, "missing destination", http.StatusBadRequest)
		return
	}

	srcPath, err := h.safePath(reqPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	dstPath, err := h.safePath(destination)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	override := overrideStr == "true"
	autoRename := renameStr == "true"

	if !override {
		if _, err := os.Stat(dstPath); err == nil {
			if autoRename {
				dstPath = generateUniqueName(dstPath)
			} else {
				http.Error(w, "destination already exists", http.StatusConflict)
				return
			}
		}
	}

	// Ensure parent of destination exists
	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	switch action {
	case "copy":
		if err := copyPath(srcPath, dstPath); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	default:
		// rename / move
		if err := os.Rename(srcPath, dstPath); err != nil {
			// Cross-device move: copy then remove
			if err := copyPath(srcPath, dstPath); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			os.RemoveAll(srcPath)
		}
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (h *Handler) handlePaste(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	reqPath := strings.TrimPrefix(r.URL.Path, "/api/paste")
	if reqPath == "" {
		reqPath = "/"
	}

	// The paste endpoint reuses the same query param logic as rename/move
	h.renameMove(w, r)
}

func (h *Handler) handleRaw(w http.ResponseWriter, r *http.Request) {
	reqPath := strings.TrimPrefix(r.URL.Path, "/api/raw")
	if reqPath == "" {
		reqPath = "/"
	}

	absPath, err := h.safePath(reqPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "not found", http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	if info.IsDir() {
		// If algo=zip is requested, create zip download
		algo := r.URL.Query().Get("algo")
		if algo == "zip" {
			w.Header().Set("Content-Type", "application/zip")
			w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.zip"`, info.Name()))
			if err := zipDir(absPath, w); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}
		http.Error(w, "is a directory", http.StatusBadRequest)
		return
	}

	inline := r.URL.Query().Get("inline") == "true"
	if inline {
		ct := mime.TypeByExtension(filepath.Ext(absPath))
		if ct == "" {
			ct = "application/octet-stream"
		}
		w.Header().Set("Content-Type", ct)
		w.Header().Set("Content-Disposition", "inline")
	}

	http.ServeFile(w, r, absPath)
}

func (h *Handler) handlePreview(w http.ResponseWriter, r *http.Request) {
	// /api/preview/{size}/{path...}
	reqPath := strings.TrimPrefix(r.URL.Path, "/api/preview/")
	parts := strings.SplitN(reqPath, "/", 2)
	if len(parts) < 2 {
		http.Error(w, "invalid preview path", http.StatusBadRequest)
		return
	}

	thumbSize := parts[0]
	filePath := "/" + parts[1]

	absPath, err := h.safePath(filePath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	if _, err := os.Stat(absPath); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	width, height := parseThumbnailSize(thumbSize)
	if err := serveThumbnail(w, absPath, width, height); err != nil {
		// Fall back to serving the original file
		http.ServeFile(w, r, absPath)
	}
}

func (h *Handler) handleNodes(w http.ResponseWriter, r *http.Request) {
	hostname, _ := os.Hostname()
	nodes := []map[string]interface{}{
		{
			"name":   hostname,
			"master": true,
		},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(nodes)
}

// handleUploadInit handles POST /upload — tus-style upload init
func (h *Handler) handleUploadInit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	storagePath := r.FormValue("storage_path")
	fileRelPath := r.FormValue("file_relative_path")
	fileSizeStr := r.FormValue("file_size")

	fileSize, _ := strconv.ParseInt(fileSizeStr, 10, 64)

	fullPath := filepath.Join(storagePath, fileRelPath)
	absPath, err := h.safePath(fullPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Create an empty file
	f, err := os.Create(absPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	f.Close()

	// Return upload ID (use the path hash as ID)
	uploadID := fmt.Sprintf("%x", time.Now().UnixNano())

	uploadsMu.Lock()
	uploadsInProgress[uploadID] = &uploadState{
		absPath:  absPath,
		fileSize: fileSize,
		offset:   0,
	}
	uploadsMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data": map[string]interface{}{
			"id":        uploadID,
			"file_size": fileSize,
			"offset":    0,
		},
	})
}

// handleUploadChunk handles PATCH /upload/{id} — tus-style chunk upload
func (h *Handler) handleUploadChunk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	uploadID := strings.TrimPrefix(r.URL.Path, "/upload/")
	if uploadID == "" {
		http.Error(w, "missing upload ID", http.StatusBadRequest)
		return
	}

	uploadsMu.Lock()
	state, ok := uploadsInProgress[uploadID]
	uploadsMu.Unlock()

	if !ok {
		http.Error(w, "unknown upload ID", http.StatusNotFound)
		return
	}

	if err := r.ParseMultipartForm(h.MaxUploadSize); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file field", http.StatusBadRequest)
		return
	}
	defer file.Close()

	offsetStr := r.FormValue("upload_offset")
	offset, _ := strconv.ParseInt(offsetStr, 10, 64)

	f, err := os.OpenFile(state.absPath, os.O_WRONLY, 0644)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer f.Close()

	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	written, err := io.Copy(f, file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	newOffset := offset + written

	uploadsMu.Lock()
	state.offset = newOffset
	if newOffset >= state.fileSize {
		delete(uploadsInProgress, uploadID)
	}
	uploadsMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"offset": newOffset,
	})
}

// handleUploadLink returns a URL for the chunked upload endpoint
func (h *Handler) handleUploadLink(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Query().Get("p")
	if p == "" {
		p = "/"
	}
	// Return the upload endpoint URL
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	host := r.Host
	uploadURL := fmt.Sprintf("%s://%s/upload", scheme, host)
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(uploadURL))
}

// handleUploadedBytes returns how many bytes have already been uploaded for a resumable upload
func (h *Handler) handleUploadedBytes(w http.ResponseWriter, r *http.Request) {
	parentDir := r.URL.Query().Get("parent_dir")
	fileName := r.URL.Query().Get("file_name")

	if parentDir == "" || fileName == "" {
		http.Error(w, "missing params", http.StatusBadRequest)
		return
	}

	fullPath := filepath.Join(parentDir, fileName)
	absPath, err := h.safePath(fullPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	info, err := os.Stat(absPath)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]int64{"uploaded_bytes": 0})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int64{"uploaded_bytes": info.Size()})
}

func parseThumbnailSize(size string) (int, int) {
	switch size {
	case "small":
		return 128, 128
	case "medium":
		return 256, 256
	case "large":
		return 512, 512
	default:
		return 256, 256
	}
}
