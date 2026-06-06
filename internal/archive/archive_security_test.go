package archive

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeRawTarGz writes a tar.gz built from the given headers/bodies verbatim
// (no prefix-stripping helper), so tests can craft malicious member names.
func writeRawTarGz(t *testing.T, entries []tar.Header, bodies map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "evil.tar.gz")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gw := gzip.NewWriter(f)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()
	for i := range entries {
		hdr := entries[i]
		body := bodies[hdr.Name]
		hdr.Size = int64(len(body))
		if err := tw.WriteHeader(&hdr); err != nil {
			t.Fatal(err)
		}
		if body != "" {
			if _, err := tw.Write([]byte(body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	return path
}

func TestExtractTarRejectsTraversal(t *testing.T) {
	// Include a benign sibling so no single leading dir is stripped and the
	// malicious "../" name reaches the boundary check intact.
	src := writeRawTarGz(t,
		[]tar.Header{
			{Name: "app/ok", Mode: 0o644},
			{Name: "../escape.txt", Mode: 0o644},
		},
		map[string]string{"app/ok": "fine", "../escape.txt": "pwned"},
	)
	parent := t.TempDir()
	dest := filepath.Join(parent, "tool")
	err := Extract(src, dest)
	if err == nil || !strings.Contains(err.Error(), "escapes destination") {
		t.Fatalf("expected escape error, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(parent, "escape.txt")); err == nil {
		t.Error("traversal file was written outside destination")
	}
}

func TestExtractTarRejectsSiblingPrefixEscape(t *testing.T) {
	// dest is .../tool; a member resolving to .../tool-evil/x shares the
	// string prefix but is NOT within dest. A raw HasPrefix check would
	// let this through; the boundary check must reject it.
	src := writeRawTarGz(t,
		[]tar.Header{
			{Name: "app/ok", Mode: 0o644},
			{Name: "../tool-evil/x", Mode: 0o644},
		},
		map[string]string{"app/ok": "fine", "../tool-evil/x": "pwned"},
	)
	parent := t.TempDir()
	dest := filepath.Join(parent, "tool")
	if err := Extract(src, dest); err == nil {
		t.Fatal("expected sibling-prefix escape to be rejected")
	}
	if _, err := os.Stat(filepath.Join(parent, "tool-evil", "x")); err == nil {
		t.Error("sibling-prefix file was written outside destination")
	}
}

func TestExtractTarRejectsSymlinkEscape(t *testing.T) {
	// An absolute symlink pointing outside dest must be rejected so a later
	// entry cannot be written through it.
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	src := writeRawTarGz(t,
		[]tar.Header{{Name: "link", Typeflag: tar.TypeSymlink, Linkname: outside, Mode: 0o777}},
		nil,
	)
	dest := t.TempDir()
	if err := Extract(src, dest); err == nil || !strings.Contains(err.Error(), "symlink escapes") {
		t.Fatalf("expected symlink escape error, got %v", err)
	}
}

func TestExtractTarRejectsRelativeSymlinkEscape(t *testing.T) {
	src := writeRawTarGz(t,
		[]tar.Header{{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "../../escape", Mode: 0o777}},
		nil,
	)
	dest := t.TempDir()
	if err := Extract(src, dest); err == nil {
		t.Fatal("expected relative symlink escape to be rejected")
	}
}

func TestExtractTarAllowsInBoundsSymlink(t *testing.T) {
	src := writeRawTarGz(t,
		[]tar.Header{
			{Name: "real", Typeflag: tar.TypeReg, Mode: 0o644},
			{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "real", Mode: 0o777},
		},
		map[string]string{"real": "content"},
	)
	dest := t.TempDir()
	if err := Extract(src, dest); err != nil {
		t.Fatalf("in-bounds symlink should be allowed, got %v", err)
	}
	if _, err := os.Lstat(filepath.Join(dest, "link")); err != nil {
		t.Errorf("expected symlink to be created: %v", err)
	}
}

func TestExtractZipRejectsTraversal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evil.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	// Benign sibling so no common prefix is stripped.
	if ok, err := zw.Create("app/ok"); err != nil {
		t.Fatal(err)
	} else if _, err := ok.Write([]byte("fine")); err != nil {
		t.Fatal(err)
	}
	w, err := zw.Create("../escape.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("pwned")); err != nil {
		t.Fatal(err)
	}
	zw.Close()
	f.Close()

	parent := t.TempDir()
	dest := filepath.Join(parent, "tool")
	if err := Extract(path, dest); err == nil || !strings.Contains(err.Error(), "escapes destination") {
		t.Fatalf("expected zip escape error, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(parent, "escape.txt")); err == nil {
		t.Error("zip traversal file was written outside destination")
	}
}

func TestExtractRejectsOversizedArchive(t *testing.T) {
	orig := maxExtractBytes
	maxExtractBytes = 8
	defer func() { maxExtractBytes = orig }()

	src := createTestTarGz(t, "", map[string]string{"big": "way more than eight bytes"})
	dest := t.TempDir()
	if err := Extract(src, dest); err == nil || !strings.Contains(err.Error(), "maximum extracted size") {
		t.Fatalf("expected size-limit error, got %v", err)
	}
}

func TestExtractRejectsTooManyEntries(t *testing.T) {
	orig := maxExtractEntries
	maxExtractEntries = 1
	defer func() { maxExtractEntries = orig }()

	src := createTestTarGz(t, "", map[string]string{"a": "x", "b": "y"})
	dest := t.TempDir()
	if err := Extract(src, dest); err == nil || !strings.Contains(err.Error(), "maximum entry count") {
		t.Fatalf("expected entry-count error, got %v", err)
	}
}

func TestXZDecompressedName(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"foo.tar.xz", "foo.tar"},
		{"foo.txz", "foo.tar"},
		{"foo.tlz", "foo.tar"},
		{"foo.xz", "foo"},
	}
	for _, tt := range tests {
		got, err := xzDecompressedName(tt.in)
		if err != nil {
			t.Errorf("xzDecompressedName(%q) error: %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("xzDecompressedName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
	if _, err := xzDecompressedName("foo.gz"); err == nil {
		t.Error("expected error for non-xz suffix")
	}
}
