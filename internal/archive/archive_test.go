package archive

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestExtractTarGz(t *testing.T) {
	// Create a tar.gz with a leading directory
	src := createTestTarGz(t, "myapp-v1.0/", map[string]string{
		"myapp-v1.0/myapp":       "binary-content",
		"myapp-v1.0/README.md":   "readme",
		"myapp-v1.0/doc/man.1":   "man page",
	})

	dest := t.TempDir()
	if err := Extract(src, dest); err != nil {
		t.Fatalf("Extract: %v", err)
	}

	// Leading dir should be stripped
	assertFileExists(t, filepath.Join(dest, "myapp"))
	assertFileExists(t, filepath.Join(dest, "README.md"))
	assertFileExists(t, filepath.Join(dest, "doc", "man.1"))

	// Leading dir itself should NOT exist
	if _, err := os.Stat(filepath.Join(dest, "myapp-v1.0")); err == nil {
		t.Error("leading directory should have been stripped")
	}
}

func TestExtractTarGzNoStrip(t *testing.T) {
	// Create a tar.gz with multiple top-level entries (no stripping)
	src := createTestTarGz(t, "", map[string]string{
		"myapp":     "binary",
		"README.md": "readme",
	})

	dest := t.TempDir()
	if err := Extract(src, dest); err != nil {
		t.Fatalf("Extract: %v", err)
	}

	assertFileExists(t, filepath.Join(dest, "myapp"))
	assertFileExists(t, filepath.Join(dest, "README.md"))
}

func TestExtractZip(t *testing.T) {
	src := createTestZip(t, "tool-v2/", map[string]string{
		"tool-v2/tool":   "binary",
		"tool-v2/doc.md": "docs",
	})

	dest := t.TempDir()
	if err := Extract(src, dest); err != nil {
		t.Fatalf("Extract: %v", err)
	}

	assertFileExists(t, filepath.Join(dest, "tool"))
	assertFileExists(t, filepath.Join(dest, "doc.md"))
}

func TestExtractBareBinary(t *testing.T) {
	// Create a plain file (non-archive)
	dir := t.TempDir()
	src := filepath.Join(dir, "mytool")
	if err := os.WriteFile(src, []byte("#!/bin/sh\necho hi"), 0o755); err != nil {
		t.Fatal(err)
	}

	dest := t.TempDir()
	if err := Extract(src, dest); err != nil {
		t.Fatalf("Extract bare binary: %v", err)
	}

	target := filepath.Join(dest, "mytool")
	assertFileExists(t, target)

	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0o111 == 0 {
		t.Error("bare binary should be executable")
	}
}

func TestExtractTarXz(t *testing.T) {
	// Skip if xz is not installed
	if _, err := exec.LookPath("xz"); err != nil {
		t.Skip("xz not installed, skipping tar.xz test")
	}

	// Create a tar first, then xz compress it
	tarPath := filepath.Join(t.TempDir(), "test.tar")
	f, err := os.Create(tarPath)
	if err != nil {
		t.Fatal(err)
	}
	tw := tar.NewWriter(f)

	// Write a directory entry
	if err := tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeDir,
		Name:     "rv-1.0/",
		Mode:     0o755,
	}); err != nil {
		t.Fatal(err)
	}

	// Write a file entry
	content := "binary-content"
	if err := tw.WriteHeader(&tar.Header{
		Name: "rv-1.0/rv",
		Mode: 0o755,
		Size: int64(len(content)),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	tw.Close()
	f.Close()

	// Compress with xz
	cmd := exec.Command("xz", tarPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("xz compress: %s: %v", out, err)
	}

	xzPath := tarPath + ".xz"
	dest := t.TempDir()
	if err := Extract(xzPath, dest); err != nil {
		t.Fatalf("Extract tar.xz: %v", err)
	}

	// Leading dir should be stripped
	assertFileExists(t, filepath.Join(dest, "rv"))

	// Leading dir itself should NOT exist
	if _, err := os.Stat(filepath.Join(dest, "rv-1.0")); err == nil {
		t.Error("leading directory should have been stripped")
	}
}

// --- helpers ---

func createTestTarGz(t *testing.T, prefix string, files map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.tar.gz")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	// Write directory entries for prefix if present
	if prefix != "" {
		if err := tw.WriteHeader(&tar.Header{
			Typeflag: tar.TypeDir,
			Name:     prefix,
			Mode:     0o755,
		}); err != nil {
			t.Fatal(err)
		}
	}

	for name, content := range files {
		hdr := &tar.Header{
			Name: name,
			Mode: 0o755,
			Size: int64(len(content)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}

	return path
}

func createTestZip(t *testing.T, prefix string, files map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	defer zw.Close()

	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}

	return path
}

func assertFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected file to exist: %s", path)
	}
}
