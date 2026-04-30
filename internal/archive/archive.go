package archive

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Extract unpacks an archive file into destDir.
// Supports .tar.gz, .tgz, .tar.xz, .txz, and .zip formats.
// If the archive has a single top-level directory, its contents are promoted up
// (the leading directory is stripped).
// For non-archive files (bare binaries), the file is copied directly and made executable.
func Extract(archivePath, destDir string) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}

	lower := strings.ToLower(archivePath)
	switch {
	case strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz"):
		return extractTarGz(archivePath, destDir)
	case strings.HasSuffix(lower, ".tar.xz") || strings.HasSuffix(lower, ".txz"):
		return extractTarXz(archivePath, destDir)
	case strings.HasSuffix(lower, ".zip"):
		return extractZip(archivePath, destDir)
	default:
		return copyBinary(archivePath, destDir)
	}
}

func extractTarGz(archivePath, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("gzip: %w", err)
	}
	defer gz.Close()

	prefix := detectTarPrefix(archivePath)
	return extractTar(gz, destDir, prefix)
}

func extractTarXz(archivePath, destDir string) error {
	// xz decompression requires the xz command since Go stdlib doesn't include it
	if _, err := exec.LookPath("xz"); err != nil {
		return fmt.Errorf("xz command not found (required for .tar.xz): %w", err)
	}

	// Decompress to a temporary .tar file
	tarPath := strings.TrimSuffix(archivePath, filepath.Ext(archivePath))
	if tarPath == archivePath {
		tarPath = archivePath + ".tar"
	}

	// Run xz -dk to decompress (keep original, force overwrite)
	cmd := exec.Command("xz", "-dkf", archivePath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("xz decompress: %s: %w", string(out), err)
	}
	defer os.Remove(tarPath)

	f, err := os.Open(tarPath)
	if err != nil {
		return err
	}
	defer f.Close()

	prefix := detectTarPrefixFromReader(f)
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	return extractTar(f, destDir, prefix)
} // extractTar reads tar entries from r and writes them to destDir, stripping prefix.
func extractTar(r io.Reader, destDir, prefix string) error {
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar: %w", err)
		}

		name := hdr.Name
		if prefix != "" {
			name = strings.TrimPrefix(name, prefix)
			if name == "" {
				continue
			}
		}

		target := filepath.Join(destDir, filepath.FromSlash(name))

		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(destDir)) {
			return fmt.Errorf("tar entry escapes destination: %s", hdr.Name)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			mode := hdr.FileInfo().Mode()
			if err := writeFile(target, tr, mode); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			_ = os.Remove(target)
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				return err
			}
		}
	}
	return nil
}

func detectTarPrefixFromReader(r io.Reader) string {
	tr := tar.NewReader(r)
	topDirs := make(map[string]bool)
	for {
		hdr, err := tr.Next()
		if err != nil {
			break
		}
		parts := strings.SplitN(filepath.ToSlash(hdr.Name), "/", 2)
		if len(parts) > 0 && parts[0] != "." {
			topDirs[parts[0]] = true
		}
	}
	if len(topDirs) == 1 {
		for dir := range topDirs {
			return dir + "/"
		}
	}
	return ""
}

// detectTarPrefix returns the leading directory shared by all entries in a
// tar.gz archive, or "" when there is no single common prefix.
func detectTarPrefix(archivePath string) string {
	f, err := os.Open(archivePath)
	if err != nil {
		return ""
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return ""
	}
	defer gz.Close()

	return detectTarPrefixFromReader(gz)
}

func extractZip(archivePath, destDir string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("zip: %w", err)
	}
	defer r.Close()

	prefix := detectZipPrefix(r)

	for _, f := range r.File {
		name := f.Name
		if prefix != "" {
			name = strings.TrimPrefix(name, prefix)
			if name == "" {
				continue
			}
		}

		target := filepath.Join(destDir, filepath.FromSlash(name))

		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(destDir)) {
			return fmt.Errorf("zip entry escapes destination: %s", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}
		mode := f.FileInfo().Mode()
		err = writeFile(target, rc, mode)
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func detectZipPrefix(r *zip.ReadCloser) string {
	topDirs := make(map[string]bool)
	for _, f := range r.File {
		parts := strings.SplitN(filepath.ToSlash(f.Name), "/", 2)
		if len(parts) > 0 && parts[0] != "." {
			topDirs[parts[0]] = true
		}
	}
	if len(topDirs) == 1 {
		for dir := range topDirs {
			return dir + "/"
		}
	}
	return ""
}

// copyBinary handles non-archive assets (bare binaries like jq releases).
func copyBinary(src, destDir string) error {
	name := filepath.Base(src)
	target := filepath.Join(destDir, name)

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	return writeFile(target, in, 0o755)
}

func writeFile(path string, r io.Reader, mode os.FileMode) error {
	out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, r)
	return err
}
