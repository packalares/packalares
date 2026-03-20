package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// FileHandler handles all file and storage operations.
type FileHandler struct {
	DataPath      string
	UploadMaxSize int64

	mu     sync.RWMutex
	mounts []MountEntry
}

// FileEntry represents a file or directory in a listing.
type FileEntry struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	IsDir    bool   `json:"is_dir"`
	Size     int64  `json:"size"`
	Modified string `json:"modified"`
	Mode     string `json:"mode"`
}

// FileInfo represents detailed file/directory information.
type FileInfo struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	IsDir    bool   `json:"is_dir"`
	Size     int64  `json:"size"`
	Modified string `json:"modified"`
	Mode     string `json:"mode"`
	Items    int    `json:"items,omitempty"` // number of items if directory
}

// MountEntry represents a mounted external storage.
type MountEntry struct {
	ID         string `json:"id"`
	Type       string `json:"type"`       // smb, nfs, rclone, s3
	Source     string `json:"source"`     // e.g. //nas/share, s3://bucket
	MountPoint string `json:"mount_point"`
	Status     string `json:"status"` // mounted, error
	MountedAt  string `json:"mounted_at"`
}

// MountRequest is the body for mount/unmount operations.
type MountRequest struct {
	Type     string `json:"type"`       // smb, nfs, rclone
	Source   string `json:"source"`     // //host/share or bucket name
	Target   string `json:"target"`     // mount point name (relative to data path)
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Options  string `json:"options,omitempty"` // extra mount options
}

// ---------------------------------------------------------------
// Path safety
// ---------------------------------------------------------------

// safePath resolves a user-supplied path and ensures it stays within DataPath.
// Returns the cleaned absolute path or an error.
func (h *FileHandler) safePath(raw string) (string, error) {
	if raw == "" {
		return h.DataPath, nil
	}

	// If the path starts with DataPath, use it directly; otherwise treat as relative
	var abs string
	if filepath.IsAbs(raw) {
		abs = filepath.Clean(raw)
	} else {
		abs = filepath.Clean(filepath.Join(h.DataPath, raw))
	}

	// Ensure the resolved path is under DataPath
	if !strings.HasPrefix(abs, h.DataPath) {
		return "", fmt.Errorf("path traversal denied")
	}
	return abs, nil
}

// ---------------------------------------------------------------
// JSON helpers
// ---------------------------------------------------------------

func jsonOK(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func jsonError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func decodeBody(r *http.Request, v interface{}) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

// ---------------------------------------------------------------
// GET /api/files/list?path=...
// ---------------------------------------------------------------

func (h *FileHandler) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}

	dir, err := h.safePath(r.URL.Query().Get("path"))
	if err != nil {
		jsonError(w, http.StatusForbidden, err.Error())
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			jsonError(w, http.StatusNotFound, "directory not found")
		} else {
			jsonError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	files := make([]FileEntry, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, FileEntry{
			Name:     e.Name(),
			Path:     filepath.Join(dir, e.Name()),
			IsDir:    e.IsDir(),
			Size:     info.Size(),
			Modified: info.ModTime().UTC().Format(time.RFC3339),
			Mode:     info.Mode().String(),
		})
	}

	// Return the relative path for breadcrumb navigation
	relPath, _ := filepath.Rel(h.DataPath, dir)
	if relPath == "." {
		relPath = ""
	}

	jsonOK(w, map[string]interface{}{
		"path":      dir,
		"rel_path":  relPath,
		"data_root": h.DataPath,
		"files":     files,
	})
}

// ---------------------------------------------------------------
// GET /api/files/download?path=...
// ---------------------------------------------------------------

func (h *FileHandler) Download(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}

	fpath, err := h.safePath(r.URL.Query().Get("path"))
	if err != nil {
		jsonError(w, http.StatusForbidden, err.Error())
		return
	}

	info, err := os.Stat(fpath)
	if err != nil {
		jsonError(w, http.StatusNotFound, "file not found")
		return
	}
	if info.IsDir() {
		jsonError(w, http.StatusBadRequest, "cannot download a directory")
		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(fpath)))
	http.ServeFile(w, r, fpath)
}

// ---------------------------------------------------------------
// POST /api/files/upload  (multipart/form-data)
//   field "path" — destination directory
//   field "file" — the file
// ---------------------------------------------------------------

func (h *FileHandler) Upload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, h.UploadMaxSize)

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		jsonError(w, http.StatusRequestEntityTooLarge, "file too large or bad form data")
		return
	}

	destDir, err := h.safePath(r.FormValue("path"))
	if err != nil {
		jsonError(w, http.StatusForbidden, err.Error())
		return
	}

	// Ensure destination directory exists
	if err := os.MkdirAll(destDir, 0755); err != nil {
		jsonError(w, http.StatusInternalServerError, "cannot create destination directory")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		jsonError(w, http.StatusBadRequest, "missing file field")
		return
	}
	defer file.Close()

	// Sanitize filename
	name := filepath.Base(header.Filename)
	if name == "" || name == "." || name == ".." {
		jsonError(w, http.StatusBadRequest, "invalid filename")
		return
	}

	destPath := filepath.Join(destDir, name)

	// Verify the destination is still within DataPath
	if !strings.HasPrefix(destPath, h.DataPath) {
		jsonError(w, http.StatusForbidden, "path traversal denied")
		return
	}

	dst, err := os.Create(destPath)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "cannot create file: "+err.Error())
		return
	}
	defer dst.Close()

	written, err := io.Copy(dst, file)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "write error: "+err.Error())
		return
	}

	log.Printf("uploaded %s (%d bytes) to %s", name, written, destPath)
	jsonOK(w, map[string]interface{}{
		"name": name,
		"path": destPath,
		"size": written,
	})
}

// ---------------------------------------------------------------
// POST /api/files/mkdir   { "path": "/packalares/data/newdir" }
// ---------------------------------------------------------------

func (h *FileHandler) Mkdir(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}

	var body struct {
		Path string `json:"path"`
	}
	if err := decodeBody(r, &body); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	dir, err := h.safePath(body.Path)
	if err != nil {
		jsonError(w, http.StatusForbidden, err.Error())
		return
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		jsonError(w, http.StatusInternalServerError, "mkdir failed: "+err.Error())
		return
	}

	jsonOK(w, map[string]string{"path": dir, "status": "created"})
}

// ---------------------------------------------------------------
// POST /api/files/delete  { "path": "..." }
// ---------------------------------------------------------------

func (h *FileHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}

	var body struct {
		Path string `json:"path"`
	}
	if err := decodeBody(r, &body); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	target, err := h.safePath(body.Path)
	if err != nil {
		jsonError(w, http.StatusForbidden, err.Error())
		return
	}

	// Prevent deleting the data root
	if target == h.DataPath {
		jsonError(w, http.StatusForbidden, "cannot delete the data root")
		return
	}

	if err := os.RemoveAll(target); err != nil {
		jsonError(w, http.StatusInternalServerError, "delete failed: "+err.Error())
		return
	}

	log.Printf("deleted %s", target)
	jsonOK(w, map[string]string{"status": "deleted", "path": target})
}

// ---------------------------------------------------------------
// POST /api/files/move  { "source": "...", "destination": "..." }
// ---------------------------------------------------------------

func (h *FileHandler) Move(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}

	var body struct {
		Source      string `json:"source"`
		Destination string `json:"destination"`
	}
	if err := decodeBody(r, &body); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	src, err := h.safePath(body.Source)
	if err != nil {
		jsonError(w, http.StatusForbidden, err.Error())
		return
	}
	dst, err := h.safePath(body.Destination)
	if err != nil {
		jsonError(w, http.StatusForbidden, err.Error())
		return
	}

	if err := os.Rename(src, dst); err != nil {
		jsonError(w, http.StatusInternalServerError, "move failed: "+err.Error())
		return
	}

	log.Printf("moved %s -> %s", src, dst)
	jsonOK(w, map[string]string{"status": "moved", "source": src, "destination": dst})
}

// ---------------------------------------------------------------
// POST /api/files/copy  { "source": "...", "destination": "..." }
// ---------------------------------------------------------------

func (h *FileHandler) Copy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}

	var body struct {
		Source      string `json:"source"`
		Destination string `json:"destination"`
	}
	if err := decodeBody(r, &body); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	src, err := h.safePath(body.Source)
	if err != nil {
		jsonError(w, http.StatusForbidden, err.Error())
		return
	}
	dst, err := h.safePath(body.Destination)
	if err != nil {
		jsonError(w, http.StatusForbidden, err.Error())
		return
	}

	info, err := os.Stat(src)
	if err != nil {
		jsonError(w, http.StatusNotFound, "source not found")
		return
	}

	if info.IsDir() {
		if err := copyDir(src, dst); err != nil {
			jsonError(w, http.StatusInternalServerError, "copy failed: "+err.Error())
			return
		}
	} else {
		if err := copyFile(src, dst); err != nil {
			jsonError(w, http.StatusInternalServerError, "copy failed: "+err.Error())
			return
		}
	}

	log.Printf("copied %s -> %s", src, dst)
	jsonOK(w, map[string]string{"status": "copied", "source": src, "destination": dst})
}

// ---------------------------------------------------------------
// GET /api/files/info?path=...
// ---------------------------------------------------------------

func (h *FileHandler) Info(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}

	fpath, err := h.safePath(r.URL.Query().Get("path"))
	if err != nil {
		jsonError(w, http.StatusForbidden, err.Error())
		return
	}

	info, err := os.Stat(fpath)
	if err != nil {
		if os.IsNotExist(err) {
			jsonError(w, http.StatusNotFound, "not found")
		} else {
			jsonError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	result := FileInfo{
		Name:     info.Name(),
		Path:     fpath,
		IsDir:    info.IsDir(),
		Size:     info.Size(),
		Modified: info.ModTime().UTC().Format(time.RFC3339),
		Mode:     info.Mode().String(),
	}

	if info.IsDir() {
		// Count items in directory
		entries, err := os.ReadDir(fpath)
		if err == nil {
			result.Items = len(entries)
		}
		// Calculate total size
		result.Size = dirSize(fpath)
	}

	jsonOK(w, result)
}

// ---------------------------------------------------------------
// GET /api/storage/mounts
// ---------------------------------------------------------------

func (h *FileHandler) ListMounts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}

	h.mu.RLock()
	mounts := make([]MountEntry, len(h.mounts))
	copy(mounts, h.mounts)
	h.mu.RUnlock()

	jsonOK(w, map[string]interface{}{"mounts": mounts})
}

// ---------------------------------------------------------------
// POST /api/storage/mount
// ---------------------------------------------------------------

func (h *FileHandler) Mount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}

	var req MountRequest
	if err := decodeBody(r, &req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Type == "" || req.Source == "" || req.Target == "" {
		jsonError(w, http.StatusBadRequest, "type, source, and target are required")
		return
	}

	// The mount point is always under DataPath
	mountPoint, err := h.safePath(req.Target)
	if err != nil {
		jsonError(w, http.StatusForbidden, err.Error())
		return
	}

	// Create mount point directory
	if err := os.MkdirAll(mountPoint, 0755); err != nil {
		jsonError(w, http.StatusInternalServerError, "cannot create mount point")
		return
	}

	var cmd *exec.Cmd
	switch req.Type {
	case "smb", "cifs":
		opts := "rw"
		if req.Username != "" {
			opts += fmt.Sprintf(",username=%s,password=%s", req.Username, req.Password)
		} else {
			opts += ",guest"
		}
		if req.Options != "" {
			opts += "," + req.Options
		}
		cmd = exec.Command("mount", "-t", "cifs", req.Source, mountPoint, "-o", opts)

	case "nfs":
		opts := "rw"
		if req.Options != "" {
			opts += "," + req.Options
		}
		cmd = exec.Command("mount", "-t", "nfs", req.Source, mountPoint, "-o", opts)

	case "rclone", "s3":
		// Use rclone mount for S3, Google Drive, etc.
		args := []string{"mount", req.Source, mountPoint, "--daemon", "--allow-non-empty", "--vfs-cache-mode", "full"}
		if req.Options != "" {
			for _, opt := range strings.Split(req.Options, ",") {
				args = append(args, "--"+strings.TrimSpace(opt))
			}
		}
		cmd = exec.Command("rclone", args...)

	default:
		jsonError(w, http.StatusBadRequest, "unsupported mount type: "+req.Type)
		return
	}

	output, err := cmd.CombinedOutput()
	status := "mounted"
	if err != nil {
		log.Printf("mount failed: %v: %s", err, string(output))
		status = "error"
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("mount failed: %s", strings.TrimSpace(string(output))))
		return
	}

	entry := MountEntry{
		ID:         fmt.Sprintf("%s-%d", req.Type, time.Now().UnixNano()),
		Type:       req.Type,
		Source:     req.Source,
		MountPoint: mountPoint,
		Status:     status,
		MountedAt:  time.Now().UTC().Format(time.RFC3339),
	}

	h.mu.Lock()
	h.mounts = append(h.mounts, entry)
	h.mu.Unlock()

	log.Printf("mounted %s %s at %s", req.Type, req.Source, mountPoint)
	jsonOK(w, entry)
}

// ---------------------------------------------------------------
// POST /api/storage/unmount  { "id": "..." }
// ---------------------------------------------------------------

func (h *FileHandler) Unmount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}

	var body struct {
		ID string `json:"id"`
	}
	if err := decodeBody(r, &body); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	h.mu.Lock()
	var target *MountEntry
	var idx int
	for i := range h.mounts {
		if h.mounts[i].ID == body.ID {
			target = &h.mounts[i]
			idx = i
			break
		}
	}

	if target == nil {
		h.mu.Unlock()
		jsonError(w, http.StatusNotFound, "mount not found")
		return
	}

	mountPoint := target.MountPoint
	h.mounts = append(h.mounts[:idx], h.mounts[idx+1:]...)
	h.mu.Unlock()

	cmd := exec.Command("umount", mountPoint)
	if output, err := cmd.CombinedOutput(); err != nil {
		// Try lazy unmount as fallback
		cmd = exec.Command("umount", "-l", mountPoint)
		if output2, err2 := cmd.CombinedOutput(); err2 != nil {
			log.Printf("unmount failed: %v: %s / %s", err2, string(output), string(output2))
			jsonError(w, http.StatusInternalServerError, "unmount failed")
			return
		}
	}

	log.Printf("unmounted %s", mountPoint)
	jsonOK(w, map[string]string{"status": "unmounted", "id": body.ID})
}

// ---------------------------------------------------------------
// Utility functions
// ---------------------------------------------------------------

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func copyDir(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, e := range entries {
		srcPath := filepath.Join(src, e.Name())
		dstPath := filepath.Join(dst, e.Name())

		if e.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

func dirSize(path string) int64 {
	var size int64
	filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size
}

// diskUsage returns total and free bytes for the filesystem containing a path.
func diskUsage(path string) (total, free uint64) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0
	}
	total = stat.Blocks * uint64(stat.Bsize)
	free = stat.Bavail * uint64(stat.Bsize)
	return
}
