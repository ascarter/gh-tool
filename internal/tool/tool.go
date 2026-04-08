package tool

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/BurntSushi/toml"
	gh "github.com/cli/go-gh/v2"

	"github.com/ascarter/gh-tool/internal/archive"
	"github.com/ascarter/gh-tool/internal/config"
	"github.com/ascarter/gh-tool/internal/paths"
)

// InstalledState tracks what version/tag is installed for a tool.
type InstalledState struct {
	Repo    string `toml:"repo"`
	Tag     string `toml:"tag"`
	Pattern string `toml:"pattern"`
}

// Manager handles tool installation, removal, and state management.
type Manager struct {
	Dirs paths.Dirs
}

// NewManager creates a Manager with resolved XDG paths.
func NewManager(dirs paths.Dirs) *Manager {
	return &Manager{Dirs: dirs}
}

// Install downloads, extracts, and symlinks a tool.
// If verify is true, attempts attestation verification on the downloaded asset.
func (m *Manager) Install(t config.Tool, verify bool) error {
	name := t.Name()

	// Ensure directories exist
	if err := m.Dirs.EnsureDirs(); err != nil {
		return fmt.Errorf("creating directories: %w", err)
	}

	// Resolve tag
	tag := t.Tag
	if tag == "" || tag == "latest" {
		tag = ""
	}

	// Download to cache
	cacheDir := m.Dirs.CacheDir(name)
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return fmt.Errorf("creating cache dir: %w", err)
	}

	args := []string{"release", "download", "-R", t.Repo, "-D", cacheDir, "--clobber"}
	if t.Pattern != "" {
		args = append(args, "-p", t.Pattern)
	}
	if tag != "" {
		args = append(args, tag)
	}

	fmt.Printf("Downloading %s...\n", t.Repo)
	if _, _, err := gh.Exec(args...); err != nil {
		return fmt.Errorf("downloading release: %w", err)
	}

	// Find the downloaded file
	assetPath, err := findDownloadedAsset(cacheDir)
	if err != nil {
		return err
	}

	// Verify attestation if requested
	if verify {
		verifyAttestation(t.Repo, assetPath)
	}

	// Resolve the tag that was actually downloaded
	if tag == "" {
		resolved, resolveErr := resolveLatestTag(t.Repo)
		if resolveErr == nil {
			tag = resolved
		}
	}

	// Clean previous install
	toolDir := m.Dirs.ToolDir(name)
	_ = os.RemoveAll(toolDir)

	// Extract
	fmt.Printf("Extracting %s...\n", filepath.Base(assetPath))
	if err := archive.Extract(assetPath, toolDir); err != nil {
		return fmt.Errorf("extracting: %w", err)
	}

	// Create symlinks
	if err := m.createSymlinks(t, toolDir); err != nil {
		return fmt.Errorf("creating symlinks: %w", err)
	}

	// Write state
	state := InstalledState{
		Repo:    t.Repo,
		Tag:     tag,
		Pattern: t.Pattern,
	}
	if err := m.writeState(name, state); err != nil {
		return fmt.Errorf("writing state: %w", err)
	}

	fmt.Printf("✓ Installed %s", name)
	if tag != "" {
		fmt.Printf(" (%s)", tag)
	}
	fmt.Println()
	return nil
}

// Remove uninstalls a tool by removing symlinks, tool dir, cache, and state.
func (m *Manager) Remove(t config.Tool) error {
	name := t.Name()

	// Remove symlinks
	m.removeSymlinks(t)

	// Remove tool directory
	_ = os.RemoveAll(m.Dirs.ToolDir(name))

	// Remove cache
	_ = os.RemoveAll(m.Dirs.CacheDir(name))

	// Remove state
	_ = os.Remove(m.Dirs.StateFile(name))

	fmt.Printf("✓ Removed %s\n", name)
	return nil
}

// ReadState returns the installed state for a tool, or nil if not installed.
func (m *Manager) ReadState(name string) *InstalledState {
	path := m.Dirs.StateFile(name)
	state := &InstalledState{}
	if _, err := toml.DecodeFile(path, state); err != nil {
		return nil
	}
	return state
}

// LatestTag returns the latest release tag for a repo.
func LatestTag(repo string) (string, error) {
	return resolveLatestTag(repo)
}

func (m *Manager) createSymlinks(t config.Tool, toolDir string) error {
	name := t.Name()
	bins := t.Bin
	if len(bins) == 0 {
		bins = []string{name}
	}

	// Bin symlinks
	for _, bin := range bins {
		src := findFileInDir(toolDir, bin)
		if src == "" {
			return fmt.Errorf("binary %q not found in extracted files", bin)
		}
		dst := filepath.Join(m.Dirs.BinDir(), filepath.Base(bin))
		if err := forceSymlink(src, dst); err != nil {
			return fmt.Errorf("symlink %s: %w", bin, err)
		}
	}

	// Man symlinks
	for _, man := range t.Man {
		src := findFileInDir(toolDir, man)
		if src == "" {
			fmt.Printf("  warning: man page %q not found\n", man)
			continue
		}
		dst := filepath.Join(m.Dirs.ManDir(), filepath.Base(man))
		_ = forceSymlink(src, dst)
	}

	// Completion symlinks
	for _, comp := range t.Completions {
		src := findFileInDir(toolDir, comp)
		if src == "" {
			fmt.Printf("  warning: completion %q not found\n", comp)
			continue
		}
		base := filepath.Base(comp)
		// Zsh completions typically start with _
		if strings.HasPrefix(base, "_") {
			dst := filepath.Join(m.Dirs.ZshCompletionDir(), base)
			_ = forceSymlink(src, dst)
		} else {
			dst := filepath.Join(m.Dirs.BashCompletionDir(), base)
			_ = forceSymlink(src, dst)
		}
	}

	return nil
}

func (m *Manager) removeSymlinks(t config.Tool) {
	name := t.Name()
	bins := t.Bin
	if len(bins) == 0 {
		bins = []string{name}
	}

	for _, bin := range bins {
		_ = os.Remove(filepath.Join(m.Dirs.BinDir(), filepath.Base(bin)))
	}
	for _, man := range t.Man {
		_ = os.Remove(filepath.Join(m.Dirs.ManDir(), filepath.Base(man)))
	}
	for _, comp := range t.Completions {
		base := filepath.Base(comp)
		_ = os.Remove(filepath.Join(m.Dirs.ZshCompletionDir(), base))
		_ = os.Remove(filepath.Join(m.Dirs.BashCompletionDir(), base))
	}
}

func (m *Manager) writeState(name string, state InstalledState) error {
	path := m.Dirs.StateFile(name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(state)
}

func resolveLatestTag(repo string) (string, error) {
	stdout, _, err := gh.Exec("release", "view", "-R", repo, "--json", "tagName", "--jq", ".tagName")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(stdout.String()), nil
}

// findDownloadedAsset finds the first non-checksum file in a cache dir.
func findDownloadedAsset(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		// Skip checksum/signature files
		lower := strings.ToLower(name)
		if strings.Contains(lower, "checksum") || strings.Contains(lower, "sha256") ||
			strings.HasSuffix(lower, ".sig") || strings.HasSuffix(lower, ".asc") {
			continue
		}
		return filepath.Join(dir, name), nil
	}
	return "", fmt.Errorf("no asset found in %s", dir)
}

// findFileInDir searches for a file by relative path or basename within a directory tree.
func findFileInDir(root, name string) string {
	// Try exact relative path first
	exact := filepath.Join(root, name)
	if _, err := os.Stat(exact); err == nil {
		return exact
	}

	// Search by basename
	base := filepath.Base(name)
	var found string
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if filepath.Base(path) == base {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

func forceSymlink(src, dst string) error {
	_ = os.Remove(dst)
	return os.Symlink(src, dst)
}

// verifyAttestation attempts to verify a downloaded asset using gh attestation verify.
// This is best-effort: it prints a warning if verification fails but does not return an error.
func verifyAttestation(repo, assetPath string) {
	fmt.Printf("Verifying attestation for %s...\n", filepath.Base(assetPath))
	_, _, err := gh.Exec("attestation", "verify", assetPath, "-R", repo)
	if err != nil {
		fmt.Printf("  ⚠ Attestation not verified (this is expected for most repos)\n")
	} else {
		fmt.Printf("  ✓ Attestation verified\n")
	}
}

// ExpandPattern replaces {{os}} and {{arch}} placeholders in a pattern
// with runtime-detected values.
func ExpandPattern(pattern string) string {
	os := normalizeOS(runtime.GOOS)
	arch := normalizeArch(runtime.GOARCH)
	pattern = strings.ReplaceAll(pattern, "{{os}}", os)
	pattern = strings.ReplaceAll(pattern, "{{arch}}", arch)
	return pattern
}

func normalizeOS(goos string) string {
	switch goos {
	case "darwin":
		return "darwin"
	case "linux":
		return "linux"
	case "windows":
		return "windows"
	default:
		return goos
	}
}

func normalizeArch(goarch string) string {
	switch goarch {
	case "amd64":
		return "amd64"
	case "arm64":
		return "arm64"
	case "386":
		return "i386"
	default:
		return goarch
	}
}
