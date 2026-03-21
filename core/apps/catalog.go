package apps

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/packalares/packalares/core/config"
)

type CatalogEntry struct {
	Name        string   `json:"name"`
	DisplayName string   `json:"display_name"`
	Description string   `json:"description"`
	Icon        string   `json:"icon"`
	Version     string   `json:"version"`
	HelmRepo    string   `json:"helm_repo"`
	HelmChart   string   `json:"helm_chart"`
	Category    string   `json:"category"`
	Homepage    string   `json:"homepage"`
	Tags        []string `json:"tags"`
}

var (
	catalog   []CatalogEntry
	catalogMu sync.RWMutex
)

func LoadCatalog() ([]CatalogEntry, error) {
	catalogMu.RLock()
	if catalog != nil {
		defer catalogMu.RUnlock()
		return catalog, nil
	}
	catalogMu.RUnlock()

	catalogMu.Lock()
	defer catalogMu.Unlock()

	if catalog != nil {
		return catalog, nil
	}

	cfg := config.Load()

	if cfg.CatalogURL != "" {
		entries, err := fetchRemoteCatalog(cfg.CatalogURL)
		if err == nil {
			catalog = entries
			return catalog, nil
		}
	}

	paths := []string{
		"catalog.json",
		"/app/catalog.json",
		"/etc/packalares/catalog.json",
	}

	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var entries []CatalogEntry
		if err := json.Unmarshal(data, &entries); err != nil {
			continue
		}
		catalog = entries
		return catalog, nil
	}

	return nil, fmt.Errorf("catalog not found")
}

func GetCatalogEntry(name string) (*CatalogEntry, error) {
	entries, err := LoadCatalog()
	if err != nil {
		return nil, err
	}

	for i := range entries {
		if entries[i].Name == name {
			return &entries[i], nil
		}
	}

	return nil, fmt.Errorf("app %q not found in catalog", name)
}

func ReloadCatalog() ([]CatalogEntry, error) {
	catalogMu.Lock()
	catalog = nil
	catalogMu.Unlock()
	return LoadCatalog()
}

func fetchRemoteCatalog(url string) ([]CatalogEntry, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("remote catalog returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, err
	}

	var entries []CatalogEntry
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, err
	}

	return entries, nil
}
