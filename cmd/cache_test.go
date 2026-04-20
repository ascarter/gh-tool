package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFormatSize(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{0, "    0 B "},
		{500, "  500 B "},
		{1024, "  1.0 KB"},
		{1536, "  1.5 KB"},
		{1024 * 1024, "  1.0 MB"},
		{int64(2.5 * 1024 * 1024), "  2.5 MB"},
		{1024 * 1024 * 1024, "  1.0 GB"},
	}
	for _, tt := range tests {
		if got := formatSize(tt.bytes); got != tt.want {
			t.Errorf("formatSize(%d)=%q want %q", tt.bytes, got, tt.want)
		}
	}
}

func TestDirStats(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "b"), []byte("world!"), 0o644); err != nil {
		t.Fatal(err)
	}
	size, count := dirStats(root)
	if count != 2 {
		t.Errorf("count=%d want 2", count)
	}
	if size != int64(len("hello")+len("world!")) {
		t.Errorf("size=%d want 11", size)
	}
}

func TestDirStatsMissing(t *testing.T) {
	size, count := dirStats(filepath.Join(t.TempDir(), "does-not-exist"))
	if size != 0 || count != 0 {
		t.Errorf("missing dir size=%d count=%d, want 0,0", size, count)
	}
}
