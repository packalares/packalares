package files

import (
	"os"
	"path/filepath"
	"testing"
)

// newTestHandler roots a handler at <tmp>/data and creates a sibling
// <tmp>/data-backup, which shares the root's string prefix but is not inside it.
func newTestHandler(t *testing.T) (*Handler, string) {
	t.Helper()
	tmp := t.TempDir()
	dataDir := filepath.Join(tmp, "data")
	if err := os.MkdirAll(filepath.Join(dataDir, "sub"), 0o755); err != nil {
		t.Fatalf("mkdir data: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(tmp, "data-backup"), 0o755); err != nil {
		t.Fatalf("mkdir data-backup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "data-backup", "secret"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write secret: %v", err)
	}
	return &Handler{DataPath: dataDir}, tmp
}

func TestSafePathAllowsPathsInsideRoot(t *testing.T) {
	h, _ := newTestHandler(t)
	for _, in := range []string{"/", "/sub", "/sub/file.txt", "/data/sub", "/newfile.txt"} {
		if _, err := h.safePath(in); err != nil {
			t.Errorf("safePath(%q) rejected a legitimate path: %v", in, err)
		}
	}
}

func TestSafePathRejectsTraversal(t *testing.T) {
	h, _ := newTestHandler(t)
	for _, in := range []string{"/../etc/passwd", "/sub/../../etc/passwd", "/../data-backup/secret"} {
		if got, err := h.safePath(in); err == nil {
			t.Errorf("safePath(%q) allowed traversal, resolved to %q", in, got)
		}
	}
}

// A sibling directory sharing the root's prefix must not be reachable: this is
// the string-prefix bug the containment check used to have.
func TestSafePathRejectsPrefixSiblingViaSymlink(t *testing.T) {
	h, tmp := newTestHandler(t)
	link := filepath.Join(h.DataPath, "sibling")
	if err := os.Symlink(filepath.Join(tmp, "data-backup"), link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if got, err := h.safePath("/sibling/secret"); err == nil {
		t.Errorf("safePath reached prefix-sharing sibling %q", got)
	}
}

// A symlinked parent must not carry a not-yet-existing target outside the root.
// The old check resolved symlinks only when the target already existed, so
// creates and uploads escaped through a symlinked directory.
func TestSafePathRejectsWriteThroughSymlinkedParent(t *testing.T) {
	h, tmp := newTestHandler(t)
	outside := filepath.Join(tmp, "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}
	link := filepath.Join(h.DataPath, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	// Target does not exist yet — the create/upload case.
	if got, err := h.safePath("/escape/newfile.txt"); err == nil {
		t.Errorf("safePath allowed a write through a symlinked parent, resolved to %q", got)
	}
}

func TestSafePathRejectsSymlinkToOutsideFile(t *testing.T) {
	h, tmp := newTestHandler(t)
	target := filepath.Join(tmp, "outside.txt")
	if err := os.WriteFile(target, []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	link := filepath.Join(h.DataPath, "leak")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if got, err := h.safePath("/leak"); err == nil {
		t.Errorf("safePath followed a symlink out of the root to %q", got)
	}
}

// "/database/x" must not be treated as the "/data" legacy prefix plus "base/x".
func TestSafePathStripsDataPrefixBySegment(t *testing.T) {
	h, _ := newTestHandler(t)
	got, err := h.safePath("/database/x")
	if err != nil {
		t.Fatalf("safePath(/database/x): %v", err)
	}
	want := filepath.Join(h.DataPath, "database", "x")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestWithinRoot(t *testing.T) {
	cases := []struct {
		path, root string
		want       bool
	}{
		{"/data", "/data", true},
		{"/data/sub/file", "/data", true},
		{"/data-backup/secret", "/data", false},
		{"/database", "/data", false},
		{"/etc/passwd", "/data", false},
		{"/", "/data", false},
	}
	for _, c := range cases {
		if got := withinRoot(c.path, c.root); got != c.want {
			t.Errorf("withinRoot(%q, %q) = %v, want %v", c.path, c.root, got, c.want)
		}
	}
}
