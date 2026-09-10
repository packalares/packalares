package appservice

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// appNameRE matches an RFC 1123 DNS label: lowercase alphanumerics and dashes,
// starting and ending alphanumeric. App and model names are used directly as
// Helm release names and as path segments under the chart cache and app data
// directories, so anything outside this shape is invalid regardless.
var appNameRE = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// ValidateAppName rejects names that cannot safely be used as a path segment.
// Without it a name such as "../../.." collapses, via filepath.Join, to a
// directory far outside the intended root — and several call sites hand that
// result straight to os.RemoveAll.
func ValidateAppName(name string) error {
	if name == "" {
		return fmt.Errorf("name is required")
	}
	if len(name) > 63 {
		return fmt.Errorf("name %q is too long (max 63 characters)", name)
	}
	if !appNameRE.MatchString(name) {
		return fmt.Errorf("invalid name %q: expected lowercase alphanumerics and dashes", name)
	}
	return nil
}

// withinRoot reports whether path is root or lies beneath it. It compares path
// segments via filepath.Rel rather than string prefixes, so a sibling such as
// "/tmp/charts/ollama-evil" is not treated as being inside "/tmp/charts/ollama".
func withinRoot(path, root string) bool {
	path = filepath.Clean(path)
	root = filepath.Clean(root)
	if path == root {
		return true
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// childPath joins name onto root and verifies the result stays underneath it.
// It is the safe replacement for a bare filepath.Join on caller-supplied names.
func childPath(root, name string) (string, error) {
	if err := ValidateAppName(name); err != nil {
		return "", err
	}
	joined := filepath.Join(root, name)
	if !withinRoot(joined, root) || joined == filepath.Clean(root) {
		return "", fmt.Errorf("invalid name %q", name)
	}
	return joined, nil
}
