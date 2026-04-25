package auth

import (
	"bufio"
	"context"
	"log"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

// publicDomainSet is an immutable snapshot of dynamic public hosts.
// Membership lookup is O(1) via map; the pointer is swapped atomically on reload.
type publicDomainSet map[string]struct{}

// publicDomainWatcher polls a file every interval and atomically swaps the
// shared set when its content changes. No external deps (no fsnotify).
type publicDomainWatcher struct {
	path     string
	interval time.Duration
	target   *atomic.Pointer[publicDomainSet]
	lastMod  time.Time
	lastSize int64
}

// newPublicDomainWatcher constructs (but does not start) a watcher.
func newPublicDomainWatcher(path string, interval time.Duration, target *atomic.Pointer[publicDomainSet]) *publicDomainWatcher {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	return &publicDomainWatcher{
		path:     path,
		interval: interval,
		target:   target,
	}
}

// start runs the watcher loop until ctx is cancelled. It performs an immediate
// initial read so the set is populated as quickly as possible.
func (w *publicDomainWatcher) start(ctx context.Context) {
	w.reload(true)

	tick := time.NewTicker(w.interval)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			w.reload(false)
		}
	}
}

// reload re-reads the file if its mtime/size changed (or if force is true).
func (w *publicDomainWatcher) reload(force bool) {
	info, err := os.Stat(w.path)
	if err != nil {
		if os.IsNotExist(err) {
			// File missing — ensure set is empty (only on first miss to avoid log spam).
			if w.target.Load() == nil {
				empty := publicDomainSet{}
				w.target.Store(&empty)
			}
			return
		}
		log.Printf("public-domains: stat %s: %v", w.path, err)
		return
	}

	if !force && info.ModTime().Equal(w.lastMod) && info.Size() == w.lastSize {
		return
	}
	w.lastMod = info.ModTime()
	w.lastSize = info.Size()

	f, err := os.Open(w.path)
	if err != nil {
		log.Printf("public-domains: open %s: %v", w.path, err)
		return
	}
	defer f.Close()

	set := make(publicDomainSet)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		host := strings.TrimSpace(scanner.Text())
		if host == "" || strings.HasPrefix(host, "#") {
			continue
		}
		set[host] = struct{}{}
	}
	if err := scanner.Err(); err != nil {
		log.Printf("public-domains: read %s: %v", w.path, err)
		return
	}

	w.target.Store(&set)
	log.Printf("public-domains: loaded %d host(s) from %s", len(set), w.path)
}
