package discover

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	gh "github.com/cli/go-gh/v2"

	"github.com/ascarter/gh-tool/internal/archive"
)

// Layout describes what was found inside an extracted asset.
type Layout struct {
	// Executables are paths (relative to the extract root) of files with
	// the executable bit set. On Windows-style assets, .exe files are
	// included regardless of bit.
	Executables []string

	// ManPages are relative paths matching man/man[1-9]/* or *.[1-9].
	ManPages []string

	// Completions are relative paths to *.bash/*.zsh/*.fish files or files
	// under completions/{bash,zsh,fish}/.
	Completions []string
}

// DownloadAsset fetches a single named asset from a release into destDir
// using the gh CLI and returns the local path.
func DownloadAsset(repo, tag, assetName, destDir string) (string, error) {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", err
	}
	args := []string{"release", "download"}
	if tag != "" {
		args = append(args, tag)
	}
	args = append(args, "-R", repo, "-D", destDir, "-p", assetName, "--clobber")
	if _, _, err := gh.Exec(args...); err != nil {
		return "", fmt.Errorf("downloading %s: %w", assetName, err)
	}
	return filepath.Join(destDir, assetName), nil
}

// Inspect extracts the given asset into a temp directory and walks the
// resulting tree to find executables, man pages, and shell completions.
// The temp directory is removed before returning.
func Inspect(assetPath string) (*Layout, error) {
	tmp, err := os.MkdirTemp("", "gh-tool-inspect-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)

	if err := archive.Extract(assetPath, tmp); err != nil {
		return nil, fmt.Errorf("extracting %s: %w", assetPath, err)
	}
	return scanLayout(tmp)
}

// manPageRE matches files like "tool.1", "tool.8", or any path ending with
// a single-digit section suffix.
var manPageRE = regexp.MustCompile(`\.[1-9]([a-z]?)$`)

// scanLayout walks root and classifies files into executables, man pages,
// and completions.
func scanLayout(root string) (*Layout, error) {
	layout := &Layout{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		base := strings.ToLower(filepath.Base(rel))

		// Executables: top-level (or single nested dir) with exec bit, or *.exe.
		if isExecutable(path, info, base) && isTopOrSingleNested(rel) {
			layout.Executables = append(layout.Executables, rel)
		}

		// Man pages: under man/manN/ or with .N suffix, in a man-ish dir.
		if isManPage(rel) {
			layout.ManPages = append(layout.ManPages, rel)
		}

		// Completions.
		if isCompletion(rel) {
			layout.Completions = append(layout.Completions, rel)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return layout, nil
}

func isExecutable(path string, info os.FileInfo, lowerBase string) bool {
	if strings.HasSuffix(lowerBase, ".exe") {
		return true
	}
	mode := info.Mode()
	if !mode.IsRegular() {
		return false
	}
	if mode&0o111 != 0 {
		return true
	}
	// Some upstreams (notably tree-sitter/tree-sitter) ship binaries inside
	// .zip archives where the stored Unix mode lacks the execute bit. Fall
	// back to magic-byte sniffing so we still detect Mach-O / ELF / PE.
	return hasExecutableMagic(path)
}

// hasExecutableMagic reports whether the file at path begins with the
// magic bytes of a recognized native executable format (ELF, Mach-O, Mach-O
// fat / universal, Windows PE).
func hasExecutableMagic(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	var buf [4]byte
	n, _ := f.Read(buf[:])
	if n < 4 {
		return false
	}
	switch {
	case buf[0] == 0x7f && buf[1] == 'E' && buf[2] == 'L' && buf[3] == 'F':
		return true
	case buf[0] == 0xCF && buf[1] == 0xFA && buf[2] == 0xED && buf[3] == 0xFE,
		buf[0] == 0xCE && buf[1] == 0xFA && buf[2] == 0xED && buf[3] == 0xFE,
		buf[0] == 0xFE && buf[1] == 0xED && buf[2] == 0xFA && buf[3] == 0xCF,
		buf[0] == 0xFE && buf[1] == 0xED && buf[2] == 0xFA && buf[3] == 0xCE:
		return true
	case buf[0] == 0xCA && buf[1] == 0xFE && buf[2] == 0xBA && buf[3] == 0xBE,
		buf[0] == 0xBE && buf[1] == 0xBA && buf[2] == 0xFE && buf[3] == 0xCA:
		return true
	case buf[0] == 'M' && buf[1] == 'Z':
		return true
	}
	return false
}

// isTopOrSingleNested reports whether rel is a single path segment, or sits
// directly inside one nested dir. Excludes deeply nested files (libexec,
// vendor binaries, etc.).
func isTopOrSingleNested(rel string) bool {
	parts := strings.Split(rel, "/")
	return len(parts) <= 2
}

func isManPage(rel string) bool {
	low := strings.ToLower(rel)
	if !manPageRE.MatchString(low) {
		return false
	}
	if strings.Contains(low, "/man/man") || strings.HasPrefix(low, "man/man") {
		return true
	}
	// .1-.9 suffix in any of the conventional doc dirs.
	for _, dir := range []string{"man/", "doc/", "docs/", "share/man/"} {
		if strings.HasPrefix(low, dir) || strings.Contains(low, "/"+dir) {
			return true
		}
	}
	// Bare *.N file at archive root (e.g. sharkdp/fd ships fd.1 alongside
	// the binary). Only accept when there's no directory component, so we
	// don't pick up arbitrary numeric-suffixed files buried in subdirs.
	if !strings.Contains(rel, "/") {
		return true
	}
	return false
}

func isCompletion(rel string) bool {
	low := strings.ToLower(rel)
	for _, ext := range []string{".bash", ".zsh", ".fish"} {
		if strings.HasSuffix(low, ext) {
			return true
		}
	}
	// _toolname (zsh) under a completions-style dir
	if strings.Contains(low, "/completions/") || strings.HasPrefix(low, "completions/") ||
		strings.Contains(low, "/completion/") || strings.HasPrefix(low, "completion/") ||
		strings.Contains(low, "/complete/") || strings.HasPrefix(low, "complete/") ||
		strings.Contains(low, "/autocomplete/") || strings.HasPrefix(low, "autocomplete/") {
		base := filepath.Base(rel)
		if strings.HasPrefix(base, "_") {
			return true
		}
	}
	return false
}

// MatchBinName picks the executable that best matches the repo name.
// Returns the relative path of the match, or "" if no good match is found.
func (l *Layout) MatchBinName(repoName string) string {
	target := strings.ToLower(repoName)
	for _, exe := range l.Executables {
		base := strings.ToLower(filepath.Base(exe))
		// Strip .exe for matching.
		base = strings.TrimSuffix(base, ".exe")
		if base == target {
			return exe
		}
	}
	return ""
}
