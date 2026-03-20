package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strconv"
)

//go:embed index.html
var staticFS embed.FS

func main() {
	dataPath := envOr("DATA_PATH", "/packalares/data")
	uploadMaxStr := envOr("UPLOAD_MAX_SIZE", "10737418240") // 10 GB default
	listenAddr := envOr("LISTEN_ADDR", ":8080")

	uploadMax, err := strconv.ParseInt(uploadMaxStr, 10, 64)
	if err != nil {
		// Try parsing human-readable forms like "10G"
		uploadMax = parseHumanSize(uploadMaxStr)
	}

	// Ensure data directory exists
	if err := os.MkdirAll(dataPath, 0755); err != nil {
		log.Fatalf("Cannot create data directory %s: %v", dataPath, err)
	}

	h := &FileHandler{
		DataPath:      dataPath,
		UploadMaxSize: uploadMax,
	}

	mux := http.NewServeMux()

	// File operations
	mux.HandleFunc("/api/files/list", h.List)
	mux.HandleFunc("/api/files/download", h.Download)
	mux.HandleFunc("/api/files/upload", h.Upload)
	mux.HandleFunc("/api/files/mkdir", h.Mkdir)
	mux.HandleFunc("/api/files/delete", h.Delete)
	mux.HandleFunc("/api/files/move", h.Move)
	mux.HandleFunc("/api/files/copy", h.Copy)
	mux.HandleFunc("/api/files/info", h.Info)

	// Storage mount operations
	mux.HandleFunc("/api/storage/mounts", h.ListMounts)
	mux.HandleFunc("/api/storage/mount", h.Mount)
	mux.HandleFunc("/api/storage/unmount", h.Unmount)

	// Health check
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// Serve embedded UI
	indexHTML, _ := fs.Sub(staticFS, ".")
	mux.Handle("/", http.FileServer(http.FS(indexHTML)))

	log.Printf("file-manager starting on %s (data=%s, uploadMax=%d)", listenAddr, dataPath, uploadMax)
	if err := http.ListenAndServe(listenAddr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func parseHumanSize(s string) int64 {
	if len(s) == 0 {
		return 10 << 30 // 10 GB
	}
	multiplier := int64(1)
	numStr := s
	last := s[len(s)-1]
	switch last {
	case 'G', 'g':
		multiplier = 1 << 30
		numStr = s[:len(s)-1]
	case 'M', 'm':
		multiplier = 1 << 20
		numStr = s[:len(s)-1]
	case 'K', 'k':
		multiplier = 1 << 10
		numStr = s[:len(s)-1]
	case 'T', 't':
		multiplier = 1 << 40
		numStr = s[:len(s)-1]
	}
	n, err := strconv.ParseInt(numStr, 10, 64)
	if err != nil {
		return 10 << 30
	}
	return n * multiplier
}
