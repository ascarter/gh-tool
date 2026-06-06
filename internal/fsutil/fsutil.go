// Package fsutil holds small filesystem path helpers shared across packages.
package fsutil

import (
	"fmt"
	"path/filepath"
	"strings"
)

// WithinDir reports whether target is dir itself or nested inside it. It is a
// path-boundary check (separator-aware), not a raw string-prefix test, so a
// sibling like "/a/bdir" is NOT considered within "/a/b".
func WithinDir(target, dir string) bool {
	target = filepath.Clean(target)
	dir = filepath.Clean(dir)
	if target == dir {
		return true
	}
	return strings.HasPrefix(target, dir+string(filepath.Separator))
}

// SafeJoin joins name onto dir and verifies the cleaned result stays within
// dir. It returns an error for inputs that would escape dir via "..", absolute
// paths, or sibling-prefix tricks (path traversal / zip-slip).
func SafeJoin(dir, name string) (string, error) {
	target := filepath.Join(dir, filepath.FromSlash(name))
	if !WithinDir(target, dir) {
		return "", fmt.Errorf("path %q escapes %q", name, dir)
	}
	return target, nil
}
