package main

import (
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/packalares/packalares/internal/files"
)

func main() {
	dataPath := envOr("DATA_PATH", "/packalares/data")
	port := envOr("PORT", "8080")
	maxUploadStr := envOr("MAX_UPLOAD_SIZE", "10737418240") // 10 GiB
	maxUpload, _ := strconv.ParseInt(maxUploadStr, 10, 64)

	os.MkdirAll(dataPath, 0755)

	handler := files.NewHandler(dataPath, maxUpload)
	webdavHandler := files.NewWebDAVHandler(dataPath)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	mux.Handle("/webdav/", webdavHandler)
	mux.Handle("/webdav", webdavHandler)

	addr := ":" + port
	log.Printf("files-server starting on %s (data: %s)", addr, dataPath)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
