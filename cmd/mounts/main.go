package main

import (
	"log"
	"net/http"
	"os"

	"github.com/packalares/packalares/internal/mounts"
)

func main() {
	basePath := envOr("MOUNT_BASE_PATH", "/packalares/mounts")
	port := envOr("PORT", "8081")

	os.MkdirAll(basePath, 0755)

	handler := mounts.NewHandler(basePath)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Health check
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	addr := ":" + port
	log.Printf("mounts-server starting on %s (base: %s)", addr, basePath)
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
