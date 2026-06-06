// Package fsutil holds small filesystem path helpers shared across packages.
package fsutil

import (
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
