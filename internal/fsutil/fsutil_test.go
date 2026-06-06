package fsutil

import (
	"path/filepath"
	"testing"
)

func TestWithinDir(t *testing.T) {
	dir := filepath.FromSlash("/a/b")
	cases := []struct {
		target string
		want   bool
	}{
		{filepath.FromSlash("/a/b"), true},            // dir itself
		{filepath.FromSlash("/a/b/c"), true},          // nested
		{filepath.FromSlash("/a/b/c/d.txt"), true},    // deeply nested
		{filepath.FromSlash("/a/b/../b/c"), true},     // climbs but stays within after clean
		{filepath.FromSlash("/a/bdir"), false},        // sibling prefix, not within
		{filepath.FromSlash("/a"), false},             // parent
		{filepath.FromSlash("/a/c"), false},           // sibling
		{filepath.FromSlash("/a/b/../../etc"), false}, // escapes via ..
	}
	for _, c := range cases {
		if got := WithinDir(c.target, dir); got != c.want {
			t.Errorf("WithinDir(%q, %q) = %v, want %v", c.target, dir, got, c.want)
		}
	}
}
