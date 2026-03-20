package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
)

// App represents a marketplace catalog entry.
type App struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Icon        string `json:"icon"`
	Description string `json:"description"`
	Version     string `json:"version"`
	ChartURL    string `json:"chart_url"`
	Ports       []int  `json:"ports"`
	Storage     string `json:"storage"`
	GPUOptional bool   `json:"gpu_optional"`
}

// InstallRequest is the payload for install/uninstall endpoints.
type InstallRequest struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// Config holds runtime configuration sourced from environment variables.
type Config struct {
	Namespace      string
	CatalogPath    string
	MarketplaceURL string
	KubeConfig     string
}

func loadConfig() Config {
	c := Config{
		Namespace:      os.Getenv("NAMESPACE"),
		CatalogPath:    os.Getenv("CATALOG_PATH"),
		MarketplaceURL: os.Getenv("MARKETPLACE_URL"),
		KubeConfig:     os.Getenv("KUBECONFIG"),
	}
	if c.Namespace == "" {
		c.Namespace = "packalares-apps"
	}
	if c.CatalogPath == "" {
		c.CatalogPath = "/etc/packalares/catalog.json"
	}
	return c
}

func loadCatalog(path string) ([]App, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read catalog %s: %w", path, err)
	}
	var apps []App
	if err := json.Unmarshal(data, &apps); err != nil {
		return nil, fmt.Errorf("parse catalog: %w", err)
	}
	return apps, nil
}

func jsonResponse(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, status int, msg string) {
	jsonResponse(w, status, map[string]string{"error": msg})
}

func main() {
	cfg := loadConfig()

	catalog, err := loadCatalog(cfg.CatalogPath)
	if err != nil {
		log.Printf("WARNING: could not load catalog: %v", err)
		catalog = []App{}
	}
	log.Printf("Loaded %d apps from catalog", len(catalog))

	h := &Handlers{
		Config:  cfg,
		Catalog: catalog,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/status", h.Status)
	mux.HandleFunc("GET /api/metrics", h.Metrics)
	mux.HandleFunc("GET /api/apps/available", h.AvailableApps)
	mux.HandleFunc("GET /api/apps/installed", h.InstalledApps)
	mux.HandleFunc("POST /api/apps/install", h.InstallApp)
	mux.HandleFunc("POST /api/apps/uninstall", h.UninstallApp)

	addr := ":8080"
	log.Printf("app-service listening on %s (namespace=%s)", addr, cfg.Namespace)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
