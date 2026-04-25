package appservice

import (
	"context"
	"sort"
	"strings"
	"sync"
	"testing"
)

// fakeCM is an in-memory implementation of cmInterface for tests.
type fakeCM struct {
	mu        sync.Mutex
	data      map[string]string // namespace/name → body
	applyCalls int
	applyErr   error
}

func newFakeCM() *fakeCM {
	return &fakeCM{data: make(map[string]string)}
}

func (f *fakeCM) get(_ context.Context, namespace, name string) (string, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.data[namespace+"/"+name]
	return v, ok, nil
}

func (f *fakeCM) apply(_ context.Context, namespace, name, body string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.applyErr != nil {
		return f.applyErr
	}
	f.data[namespace+"/"+name] = body
	f.applyCalls++
	return nil
}

func (f *fakeCM) body(namespace, name string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.data[namespace+"/"+name]
}

func newSyncWithFake(fake *fakeCM) *PublicDomainSync {
	return &PublicDomainSync{
		cm:        fake,
		namespace: "os-framework",
		name:      "public-app-domains",
		byApp:     make(map[string][]string),
	}
}

func TestSyncEnableWritesHosts(t *testing.T) {
	fake := newFakeCM()
	s := newSyncWithFake(fake)

	if err := s.Sync(context.Background(), "gitea", true, []string{"gitea.example.com", "git.example.com"}); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	body := fake.body("os-framework", "public-app-domains")
	got := strings.Split(body, "\n")
	sort.Strings(got)
	want := []string{"git.example.com", "gitea.example.com"}
	if len(got) != len(want) {
		t.Fatalf("body lines = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("line %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSyncDisableRemovesHosts(t *testing.T) {
	fake := newFakeCM()
	s := newSyncWithFake(fake)

	_ = s.Sync(context.Background(), "gitea", true, []string{"gitea.example.com"})
	_ = s.Sync(context.Background(), "wiki", true, []string{"wiki.example.com"})

	if err := s.Sync(context.Background(), "gitea", false, nil); err != nil {
		t.Fatalf("disable: %v", err)
	}

	body := fake.body("os-framework", "public-app-domains")
	if body != "wiki.example.com" {
		t.Fatalf("body after disable = %q, want %q", body, "wiki.example.com")
	}
}

func TestSyncIdempotentSkipsApply(t *testing.T) {
	fake := newFakeCM()
	s := newSyncWithFake(fake)

	if err := s.Sync(context.Background(), "gitea", true, []string{"gitea.example.com"}); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	first := fake.applyCalls

	// Second call with same hosts must NOT re-apply.
	if err := s.Sync(context.Background(), "gitea", true, []string{"gitea.example.com"}); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if fake.applyCalls != first {
		t.Fatalf("apply calls = %d, want unchanged %d (idempotent)", fake.applyCalls, first)
	}
}

func TestSyncDedupesAndSorts(t *testing.T) {
	fake := newFakeCM()
	s := newSyncWithFake(fake)

	_ = s.Sync(context.Background(), "a", true, []string{"b.example.com", "a.example.com", "b.example.com"})

	body := fake.body("os-framework", "public-app-domains")
	want := "a.example.com\nb.example.com"
	if body != want {
		t.Fatalf("body = %q, want %q", body, want)
	}
}

func TestSyncMergesAcrossApps(t *testing.T) {
	fake := newFakeCM()
	s := newSyncWithFake(fake)

	_ = s.Sync(context.Background(), "alpha", true, []string{"alpha.example.com"})
	_ = s.Sync(context.Background(), "beta", true, []string{"beta.example.com"})

	body := fake.body("os-framework", "public-app-domains")
	want := "alpha.example.com\nbeta.example.com"
	if body != want {
		t.Fatalf("body = %q, want %q", body, want)
	}
}

func TestReconcileFromRecords(t *testing.T) {
	fake := newFakeCM()
	s := newSyncWithFake(fake)

	t.Setenv("USER_ZONE", "user.olares.local")
	t.Setenv("CUSTOM_DOMAIN", "")

	recs := []*AppRecord{
		{
			Name:         "gitea",
			PublicAccess: true,
			State:        StateRunning,
			Entrances: []Entrance{
				{Name: "gitea"},
			},
		},
		{
			Name:         "private",
			PublicAccess: false,
			State:        StateRunning,
			Entrances: []Entrance{
				{Name: "private"},
			},
		},
		{
			Name:         "uninstalled-public",
			PublicAccess: true,
			State:        StateUninstalled,
			Entrances: []Entrance{
				{Name: "uninstalled-public"},
			},
		},
	}

	if err := s.Reconcile(context.Background(), recs); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	body := fake.body("os-framework", "public-app-domains")
	want := "gitea.user.olares.local"
	if body != want {
		t.Fatalf("Reconcile body = %q, want %q", body, want)
	}
}

func TestHostsForAppWithCustomDomain(t *testing.T) {
	t.Setenv("USER_ZONE", "user.olares.local")
	t.Setenv("CUSTOM_DOMAIN", "example.com")

	rec := &AppRecord{
		Name:      "gitea",
		Entrances: []Entrance{{Name: "gitea"}, {Name: "git"}},
	}

	got := hostsForApp(rec)
	sort.Strings(got)
	want := []string{
		"git.example.com",
		"git.user.olares.local",
		"gitea.example.com",
		"gitea.user.olares.local",
	}
	if len(got) != len(want) {
		t.Fatalf("hostsForApp = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("host %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestHostsForAppFallsBackToAppName(t *testing.T) {
	t.Setenv("USER_ZONE", "user.olares.local")
	t.Setenv("CUSTOM_DOMAIN", "")

	rec := &AppRecord{Name: "solo"}
	got := hostsForApp(rec)
	if len(got) != 1 || got[0] != "solo.user.olares.local" {
		t.Fatalf("hostsForApp = %v, want [solo.user.olares.local]", got)
	}
}

func TestUniqSortedDedupes(t *testing.T) {
	got := uniqSorted([]string{"b", "a", "b", "", "  ", "a"})
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("uniqSorted = %v, want [a b]", got)
	}
}
