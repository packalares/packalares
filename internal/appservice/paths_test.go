package appservice

import (
	"path/filepath"
	"testing"
)

func TestValidateAppNameAcceptsRealNames(t *testing.T) {
	// Names taken from the shipped market catalog.
	for _, n := range []string{"ollama", "comfyuistudio", "localmaps-admin", "voice", "whisper", "gemma3-27b", "a", "a1"} {
		if err := ValidateAppName(n); err != nil {
			t.Errorf("ValidateAppName(%q) rejected a real name: %v", n, err)
		}
	}
}

func TestValidateAppNameRejectsTraversal(t *testing.T) {
	bad := []string{
		"", "..", "../../..", "../etc", "a/../../..", "/", "/etc/passwd",
		"foo/bar", "foo\\bar", ".", "-lead", "trail-", "UPPER", "under_score",
		"sp ace", "semi;colon", "null\x00byte",
	}
	for _, n := range bad {
		if err := ValidateAppName(n); err == nil {
			t.Errorf("ValidateAppName(%q) accepted an unsafe name", n)
		}
	}
}

func TestValidateAppNameRejectsOverlongName(t *testing.T) {
	long := ""
	for i := 0; i < 64; i++ {
		long += "a"
	}
	if err := ValidateAppName(long); err == nil {
		t.Error("accepted a 64-character name")
	}
}

// The core regression: filepath.Join collapses "../../.." to a directory far
// outside the root, and several call sites pass that to os.RemoveAll.
func TestChildPathContainsTraversal(t *testing.T) {
	const root = "/tmp/charts"
	if got := filepath.Join(root, "../../.."); got != "/" {
		t.Fatalf("precondition changed: Join gave %q", got)
	}
	for _, n := range []string{"../../..", "..", "../etc", "a/../../.."} {
		if got, err := childPath(root, n); err == nil {
			t.Errorf("childPath(%q, %q) escaped the root to %q", root, n, got)
		}
	}
}

func TestChildPathAllowsValidName(t *testing.T) {
	got, err := childPath("/tmp/charts", "ollama")
	if err != nil {
		t.Fatalf("childPath: %v", err)
	}
	if want := "/tmp/charts/ollama"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestWithinRoot(t *testing.T) {
	cases := []struct {
		path, root string
		want       bool
	}{
		{"/tmp/charts/ollama", "/tmp/charts", true},
		{"/tmp/charts", "/tmp/charts", true},
		{"/tmp/charts/ollama/sub/x", "/tmp/charts", true},
		// The prefix-sharing sibling that the old HasPrefix check let through.
		{"/tmp/charts/ollama-evil/x", "/tmp/charts/ollama", false},
		{"/tmp/charts-other/x", "/tmp/charts", false},
		{"/etc/passwd", "/tmp/charts", false},
		{"/", "/tmp/charts", false},
	}
	for _, c := range cases {
		if got := withinRoot(c.path, c.root); got != c.want {
			t.Errorf("withinRoot(%q, %q) = %v, want %v", c.path, c.root, got, c.want)
		}
	}
}

func TestModelStoragePathConfinement(t *testing.T) {
	// Explicit path outside the base is refused.
	for _, p := range []string{"/", "/etc", "/packalares/data", defaultModelStorageBase} {
		if got, err := modelStoragePath(ModelSpec{Name: "gemma3-27b", StoragePath: p}); err == nil {
			t.Errorf("modelStoragePath accepted %q, resolved to %q", p, got)
		}
	}
	// Inside the base is allowed.
	inside := filepath.Join(defaultModelStorageBase, "gemma3-27b")
	if got, err := modelStoragePath(ModelSpec{Name: "gemma3-27b", StoragePath: inside}); err != nil || got != inside {
		t.Errorf("modelStoragePath(%q) = %q, %v; want it allowed", inside, got, err)
	}
	// Empty falls back to the derived path.
	got, err := modelStoragePath(ModelSpec{Name: "gemma3-27b"})
	if err != nil || got != inside {
		t.Errorf("derived path = %q, %v; want %q", got, err, inside)
	}
	// A traversing model name is refused rather than derived.
	if got, err := modelStoragePath(ModelSpec{Name: "../../.."}); err == nil {
		t.Errorf("derived a path from a traversing model name: %q", got)
	}
}
