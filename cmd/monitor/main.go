package main

import (
	"log"
	"net/http"
	"os"

	"github.com/packalares/packalares/internal/monitor"
	"github.com/packalares/packalares/pkg/config"
)

func main() {
	prometheusURL := envOr("PROMETHEUS_URL", config.PrometheusURL())
	port := envOr("PORT", "8000")

	handler := monitor.NewHandler(prometheusURL)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Health check
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	addr := ":" + port
	log.Printf("monitoring-server starting on %s (prometheus: %s)", addr, prometheusURL)
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
