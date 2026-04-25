package auth

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestWatcherInitialReadAndReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "domains.txt")

	if err := os.WriteFile(path, []byte("alpha.example.com\nbeta.example.com\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	var holder atomic.Pointer[publicDomainSet]
	w := newPublicDomainWatcher(path, 50*time.Millisecond, &holder)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.start(ctx)

	// Wait for initial load.
	if !waitFor(func() bool {
		set := holder.Load()
		return set != nil && len(*set) == 2
	}, time.Second) {
		t.Fatalf("initial read did not populate set")
	}

	set := *holder.Load()
	if _, ok := set["alpha.example.com"]; !ok {
		t.Fatalf("alpha.example.com missing from set: %v", set)
	}
	if _, ok := set["beta.example.com"]; !ok {
		t.Fatalf("beta.example.com missing from set: %v", set)
	}

	// Modify the file and bump mtime so the watcher picks up the change.
	if err := os.WriteFile(path, []byte("gamma.example.com\n"), 0644); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	future := time.Now().Add(time.Second)
	_ = os.Chtimes(path, future, future)

	if !waitFor(func() bool {
		set := holder.Load()
		if set == nil {
			return false
		}
		_, hasGamma := (*set)["gamma.example.com"]
		_, hasAlpha := (*set)["alpha.example.com"]
		return hasGamma && !hasAlpha
	}, 2*time.Second) {
		t.Fatalf("watcher did not reload after rewrite, current=%v", *holder.Load())
	}
}

func TestWatcherIgnoresCommentsAndBlanks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "domains.txt")

	body := "# header\n\nfoo.example.com\n   \nbar.example.com\n"
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	var holder atomic.Pointer[publicDomainSet]
	w := newPublicDomainWatcher(path, time.Hour, &holder)
	w.reload(true)

	set := holder.Load()
	if set == nil || len(*set) != 2 {
		t.Fatalf("set = %v, want 2 entries", set)
	}
	if _, ok := (*set)["foo.example.com"]; !ok {
		t.Fatalf("foo.example.com missing")
	}
	if _, ok := (*set)["bar.example.com"]; !ok {
		t.Fatalf("bar.example.com missing")
	}
}

func TestWatcherMissingFileLeavesEmptySet(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "does-not-exist.txt")

	var holder atomic.Pointer[publicDomainSet]
	w := newPublicDomainWatcher(path, time.Hour, &holder)
	w.reload(true)

	set := holder.Load()
	if set == nil {
		t.Fatalf("expected empty set, got nil")
	}
	if len(*set) != 0 {
		t.Fatalf("expected empty set, got %v", *set)
	}
}

func waitFor(pred func() bool, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if pred() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return pred()
}
