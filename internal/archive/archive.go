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

	"github.com/ascarter/gh-tool/internal/fsutil"
)

// Extraction safety limits. Declared as vars (not consts) so tests can lower
// them to exercise the bomb guards without producing multi-gigabyte fixtures.
var (
	// maxExtractBytes caps total uncompressed bytes written per extraction,
	// guarding against decompression bombs from untrusted release assets.
	maxExtractBytes int64 = 2 << 30 // 2 GiB
	// maxExtractEntries caps the number of archive members processed.
	maxExtractEntries = 100_000
)

// Extract unpacks an archive file into destDir.
// Supports .tar.gz, .tgz, .tar.xz, .txz, and .zip formats.
// If the archive has a single top-level directory, its contents are promoted up
// (the leading directory is stripped).
// For non-archive files (bare binaries), the file is copied directly and made executable.
//
// Extraction is bounded by maxExtractBytes / maxExtractEntries, every member is
// verified to stay within destDir (no path traversal), and symlink members that
// would resolve outside destDir are rejected.
func Extract(archivePath, destDir string) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}

	g := newLimitGuard()
	lower := strings.ToLower(archivePath)
	switch {
	case strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz"):
		return extractTarGz(archivePath, destDir, g)
	case strings.HasSuffix(lower, ".tar.xz") || strings.HasSuffix(lower, ".txz"):
		return extractTarXz(archivePath, destDir, g)
	case strings.HasSuffix(lower, ".zip"):
		return extractZip(archivePath, destDir, g)
	default:
		return copyBinary(archivePath, destDir, g)
	}
}

// limitGuard tracks the remaining byte and entry budget for one extraction.
type limitGuard struct {
	bytesLeft   int64
	entriesLeft int
}

func newLimitGuard() *limitGuard {
	return &limitGuard{bytesLeft: maxExtractBytes, entriesLeft: maxExtractEntries}
}

// entry accounts for one archive member, returning an error once the entry
// budget is exhausted.
func (g *limitGuard) entry() error {
	if g.entriesLeft <= 0 {
		return fmt.Errorf("archive exceeds maximum entry count (%d)", maxExtractEntries)
	}
	g.entriesLeft--
	return nil
}

// writeFile copies r into a new file at path, honoring mode and the running
// byte budget. It errors (rather than filling the disk) if the cumulative
// extracted size would exceed maxExtractBytes.
func (g *limitGuard) writeFile(path string, r io.Reader, mode os.FileMode) error {
	out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	// Copy at most bytesLeft+1: reading the extra byte without hitting EOF
	// signals the archive is over budget.
	n, err := io.CopyN(out, r, g.bytesLeft+1)
	g.bytesLeft -= n
	if err != nil && err != io.EOF {
		return err
	}
	if g.bytesLeft < 0 {
		return fmt.Errorf("archive exceeds maximum extracted size (%d bytes)", maxExtractBytes)
	}
	return nil
}

func extractTarGz(archivePath, destDir string, g *limitGuard) error {
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
	return extractTar(gz, destDir, prefix, g)
}

func extractTarXz(archivePath, destDir string, g *limitGuard) error {
	// xz decompression requires the xz command since Go stdlib doesn't include it
	if _, err := exec.LookPath("xz"); err != nil {
		return fmt.Errorf("xz command not found (required for .tar.xz): %w", err)
	}

	// Determine the file xz will write, mirroring its suffix rules.
	tarPath, err := xzDecompressedName(archivePath)
	if err != nil {
		return err
	}

	// Run xz -dkf to decompress (keep original, force overwrite)
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
	return extractTar(f, destDir, prefix, g)
}

// xzDecompressedName returns the path `xz -d` writes when decompressing
// archivePath, mirroring xz's suffix rules: ".xz" is stripped, while the
// compact ".txz" (and ".tlz") forms map to ".tar".
func xzDecompressedName(archivePath string) (string, error) {
	low := strings.ToLower(archivePath)
	switch {
	case strings.HasSuffix(low, ".txz"):
		return archivePath[:len(archivePath)-len(".txz")] + ".tar", nil
	case strings.HasSuffix(low, ".tlz"):
		return archivePath[:len(archivePath)-len(".tlz")] + ".tar", nil
	case strings.HasSuffix(low, ".xz"):
		return archivePath[:len(archivePath)-len(".xz")], nil
	case strings.HasSuffix(low, ".lzma"):
		return archivePath[:len(archivePath)-len(".lzma")], nil
	default:
		return "", fmt.Errorf("unrecognized xz suffix: %s", archivePath)
	}
}

// extractTar reads tar entries from r and writes them to destDir, stripping prefix.
func extractTar(r io.Reader, destDir, prefix string, g *limitGuard) error {
	cleanDest := filepath.Clean(destDir)
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

		if err := g.entry(); err != nil {
			return err
		}

		// Skip the archive root itself; only real entries are extracted.
		clean := filepath.Clean(filepath.FromSlash(name))
		if clean == "." {
			continue
		}
		// Reject entries that escape destDir (Zip-Slip / path traversal).
		// Reaching the file operations below requires this strings.HasPrefix
		// containment check to pass, so it directly dominates every sink.
		target := filepath.Join(cleanDest, clean)
		if !strings.HasPrefix(target, cleanDest+string(os.PathSeparator)) {
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
			if err := g.writeFile(target, tr, mode); err != nil {
				return err
			}
		case tar.TypeSymlink:
			// Reject symlinks whose target would resolve outside destDir.
			// Without this, a later regular-file entry could be written
			// *through* the symlink and escape the install directory.
			if !symlinkWithinDir(destDir, target, hdr.Linkname) {
				return fmt.Errorf("tar symlink escapes destination: %s -> %s", hdr.Name, hdr.Linkname)
			}
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

// symlinkWithinDir reports whether a symlink created at linkPath pointing to
// linkname resolves to a location within destDir. Absolute targets and
// relative targets that climb out via ".." are rejected.
func symlinkWithinDir(destDir, linkPath, linkname string) bool {
	resolved := linkname
	if !filepath.IsAbs(linkname) {
		resolved = filepath.Join(filepath.Dir(linkPath), linkname)
	}
	return fsutil.WithinDir(resolved, destDir)
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

func extractZip(archivePath, destDir string, g *limitGuard) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("zip: %w", err)
	}
	defer r.Close()

	cleanDest := filepath.Clean(destDir)
	prefix := detectZipPrefix(r)

	for _, f := range r.File {
		name := f.Name
		if prefix != "" {
			name = strings.TrimPrefix(name, prefix)
			if name == "" {
				continue
			}
		}

		if err := g.entry(); err != nil {
			return err
		}

		// Skip the archive root itself; only real entries are extracted.
		clean := filepath.Clean(filepath.FromSlash(name))
		if clean == "." {
			continue
		}
		// Reject entries that escape destDir (Zip-Slip / path traversal).
		// Reaching the file operations below requires this strings.HasPrefix
		// containment check to pass, so it directly dominates every sink.
		target := filepath.Join(cleanDest, clean)
		if !strings.HasPrefix(target, cleanDest+string(os.PathSeparator)) {
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
		err = g.writeFile(target, rc, mode)
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
func copyBinary(src, destDir string, g *limitGuard) error {
	name := filepath.Base(src)
	target := filepath.Join(destDir, name)

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	return g.writeFile(target, in, 0o755)
}
