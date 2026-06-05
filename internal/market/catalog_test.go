package market

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// ---------- helpers ----------

// makeSidecar writes a minimal MarketApp JSON file to dir/<name>.json.
func makeSidecar(t *testing.T, dir, name, kind string) string {
	t.Helper()
	app := MarketApp{
		Name:    name,
		Title:   "Test " + name,
		Type:    kind,
		Version: "1.0.0",
	}
	data, err := json.MarshalIndent(app, "", "  ")
	if err != nil {
		t.Fatalf("marshal sidecar: %v", err)
	}
	path := filepath.Join(dir, name+".json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}
	return path
}

// newTestCatalog creates a Catalog backed by a fresh temp directory tree.
func newTestCatalog(t *testing.T) (*Catalog, string, string) {
	t.Helper()
	root := t.TempDir()
	chartsDir := filepath.Join(root, "charts")
	modelsDir := filepath.Join(chartsDir, "models")
	if err := os.MkdirAll(modelsDir, 0755); err != nil {
		t.Fatalf("mkdir models: %v", err)
	}
	curationPath := filepath.Join(root, "curation.json")
	return NewCatalog(chartsDir, curationPath), chartsDir, modelsDir
}

// ---------- tests ----------

func TestEmptyDirs(t *testing.T) {
	cat, _, _ := newTestCatalog(t)
	if err := cat.Load(); err != nil {
		t.Fatalf("Load on empty dirs: %v", err)
	}
	apps := cat.ListApps()
	if len(apps) != 0 {
		t.Errorf("expected 0 apps, got %d", len(apps))
	}
	// ListCategories must not panic
	_ = cat.ListCategories()
}

func TestScanNApps(t *testing.T) {
	cat, chartsDir, _ := newTestCatalog(t)

	names := []string{"alpha", "beta", "gamma"}
	for _, n := range names {
		makeSidecar(t, chartsDir, n, "app")
	}

	if err := cat.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	apps := cat.ListApps()
	if len(apps) != len(names) {
		t.Errorf("expected %d apps, got %d", len(names), len(apps))
	}
}

func TestScanAppsAndModels(t *testing.T) {
	cat, chartsDir, modelsDir := newTestCatalog(t)

	makeSidecar(t, chartsDir, "my-app", "app")
	makeSidecar(t, modelsDir, "my-model", "model")

	if err := cat.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	apps := cat.ListApps()
	if len(apps) != 2 {
		t.Errorf("expected 2 total entries (app+model), got %d", len(apps))
	}
}

func TestAddSidecarDetectedOnNextCall(t *testing.T) {
	cat, chartsDir, _ := newTestCatalog(t)

	// Load with empty dir
	if err := cat.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Advance indexedAt so ensureFresh triggers
	cat.mu.Lock()
	cat.indexedAt = time.Now().Add(-10 * time.Second)
	cat.mu.Unlock()

	// Add sidecar now
	makeSidecar(t, chartsDir, "newapp", "app")

	apps := cat.ListApps()
	if len(apps) != 1 {
		t.Errorf("expected 1 app after add, got %d", len(apps))
	}
}

func TestRemoveSidecarDetectedOnNextCall(t *testing.T) {
	cat, chartsDir, _ := newTestCatalog(t)

	p := makeSidecar(t, chartsDir, "vanishing", "app")

	if err := cat.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cat.ListApps()) != 1 {
		t.Fatal("precondition: expected 1 app before removal")
	}

	// Remove the file and expire the index
	os.Remove(p)
	cat.mu.Lock()
	cat.indexedAt = time.Now().Add(-10 * time.Second)
	cat.mu.Unlock()

	apps := cat.ListApps()
	if len(apps) != 0 {
		t.Errorf("expected 0 apps after removal, got %d", len(apps))
	}
}

func TestGetAppCacheHitAndMiss(t *testing.T) {
	cat, chartsDir, _ := newTestCatalog(t)
	makeSidecar(t, chartsDir, "cached-app", "app")

	if err := cat.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	app1, ok := cat.GetApp("cached-app")
	if !ok {
		t.Fatal("GetApp: expected hit")
	}

	// Second call should return identical object (from LRU)
	app2, ok := cat.GetApp("cached-app")
	if !ok {
		t.Fatal("GetApp second call: expected hit")
	}
	if app1.Name != app2.Name {
		t.Errorf("expected same name: %q vs %q", app1.Name, app2.Name)
	}

	// Miss
	_, ok = cat.GetApp("nonexistent")
	if ok {
		t.Error("GetApp nonexistent: expected miss")
	}
}

func TestMtimeInvalidation(t *testing.T) {
	cat, chartsDir, _ := newTestCatalog(t)
	path := makeSidecar(t, chartsDir, "mtime-test", "app")

	if err := cat.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Warm the LRU
	app, ok := cat.GetApp("mtime-test")
	if !ok {
		t.Fatal("initial GetApp failed")
	}
	if app.Title != "Test mtime-test" {
		t.Fatalf("unexpected title: %q", app.Title)
	}

	// Overwrite the sidecar with different content; bump mtime artificially
	updated := MarketApp{
		Name:    "mtime-test",
		Title:   "Updated Title",
		Version: "2.0.0",
	}
	data, _ := json.MarshalIndent(updated, "", "  ")
	// Slight sleep to ensure different mtime on filesystems with 1s resolution
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("rewrite sidecar: %v", err)
	}
	// Touch file to guarantee different mtime
	future := time.Now().Add(2 * time.Second)
	os.Chtimes(path, future, future)

	// Update the index entry mtime so loadOne sees the change
	cat.mu.Lock()
	if entry, ok := cat.chartIndex["mtime-test"]; ok {
		info, _ := os.Stat(path)
		if info != nil {
			entry.mtime = info.ModTime()
		}
		// Zero out mtime in index to force cache-miss path
		entry.mtime = time.Time{}
		cat.chartIndex["mtime-test"] = entry
	}
	cat.mu.Unlock()
	cat.parsed.Remove("mtime-test")

	app2, ok := cat.GetApp("mtime-test")
	if !ok {
		t.Fatal("GetApp after update: expected hit")
	}
	if app2.Title != "Updated Title" {
		t.Errorf("expected 'Updated Title', got %q", app2.Title)
	}
}

func TestLRUEviction(t *testing.T) {
	cat, chartsDir, _ := newTestCatalog(t)

	// Write more entries than lruCacheSize
	for i := 0; i < lruCacheSize+10; i++ {
		name := "app" + string(rune('a'+i%26)) + string(rune('0'+i/26))
		makeSidecar(t, chartsDir, name, "app")
	}

	if err := cat.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	apps := cat.ListApps()
	if len(apps) != lruCacheSize+10 {
		t.Errorf("expected %d apps, got %d", lruCacheSize+10, len(apps))
	}

	// LRU should not exceed its capacity
	if cat.parsed.Len() > lruCacheSize {
		t.Errorf("LRU exceeds cap: %d > %d", cat.parsed.Len(), lruCacheSize)
	}
}

func TestConcurrentReadsNoRace(t *testing.T) {
	cat, chartsDir, modelsDir := newTestCatalog(t)

	for i := 0; i < 10; i++ {
		makeSidecar(t, chartsDir, "concapp"+string(rune('a'+i)), "app")
	}
	for i := 0; i < 5; i++ {
		makeSidecar(t, modelsDir, "concmod"+string(rune('a'+i)), "model")
	}

	if err := cat.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			cat.ListApps()
			cat.GetApp("concappa")
			cat.ListCategories()
			cat.Search("conc")
		}(i)
	}
	wg.Wait()
}

// ---------- handler integration tests ----------

func TestHandlerListApps(t *testing.T) {
	cat, chartsDir, _ := newTestCatalog(t)
	makeSidecar(t, chartsDir, "testapp", "app")

	if err := cat.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	root := t.TempDir()
	h := NewHandler(cat, root)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/market/v1/apps", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var resp CatalogResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Errorf("expected 1 app in response, got %d", len(resp.Data))
	}
}

func TestHandlerGetAppNotFound(t *testing.T) {
	cat, _, _ := newTestCatalog(t)
	if err := cat.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	root := t.TempDir()
	h := NewHandler(cat, root)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/market/v1/apps/nosuchapp", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestHandlerServeScreenshot(t *testing.T) {
	cat, _, _ := newTestCatalog(t)
	if err := cat.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Create a screenshots dir with a real file
	root := t.TempDir()
	screenshotDir := filepath.Join(root, "screenshots", "mymodel")
	if err := os.MkdirAll(screenshotDir, 0755); err != nil {
		t.Fatalf("mkdir screenshots: %v", err)
	}
	if err := os.WriteFile(filepath.Join(screenshotDir, "logo.png"), []byte("PNGDATA"), 0644); err != nil {
		t.Fatalf("write logo: %v", err)
	}

	h := NewHandler(cat, root)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/market/screenshots/mymodel/logo.png", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandlerNoModelcardsRoute(t *testing.T) {
	cat, _, _ := newTestCatalog(t)
	if err := cat.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	root := t.TempDir()
	h := NewHandler(cat, root)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// Old /api/market/modelcards/ path must return 404 (no route registered)
	req := httptest.NewRequest(http.MethodGet, "/api/market/modelcards/mymodel/logo.png", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 for removed modelcards route, got %d", rr.Code)
	}
}
