package tool

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	gh "github.com/cli/go-gh/v2"

	"github.com/ascarter/gh-tool/internal/archive"
	"github.com/ascarter/gh-tool/internal/config"
	"github.com/ascarter/gh-tool/internal/paths"
)

// InstalledState records the per-machine install of a tool. It is the
// authoritative inventory entry used by list, remove, and upgrade. Only
// fields meaningful on the host where the install lives are stored.
type InstalledState struct {
	Repo        string   `toml:"repo"`
	Tag         string   `toml:"tag"`
	Pattern     string   `toml:"pattern,omitempty"` // resolved+expanded pattern actually downloaded
	Bin         []string `toml:"bin,omitempty"`
	Man         []string `toml:"man,omitempty"`
	Completions []string `toml:"completions,omitempty"`
	InstalledAt string   `toml:"installed_at,omitempty"` // RFC3339 UTC
}

// AsTool returns a config.Tool reconstructed from the installed state.
// Used to drive upgrade/reinstall without consulting the manifest. The
// resolved pattern is reused as-is; the host platform is unchanged.
func (s InstalledState) AsTool() config.Tool {
	return config.Tool{
		Repo:        s.Repo,
		Pattern:     s.Pattern,
		Tag:         s.Tag,
		Bin:         s.Bin,
		Man:         s.Man,
		Completions: s.Completions,
	}
}

// Manager handles tool installation, removal, and state management.
type Manager struct {
	Dirs paths.Dirs
}

// NewManager creates a Manager with resolved XDG paths.
func NewManager(dirs paths.Dirs) *Manager {
	return &Manager{Dirs: dirs}
}

// Install downloads, extracts, and symlinks a tool. The given config.Tool
// should carry the *source* spec (raw Pattern/Patterns from the manifest or
// CLI flags); Install resolves the platform-specific pattern and expands
// template variables internally. If verify is true, attempts attestation
// verification on the downloaded asset.
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

	// Resolve and expand the source pattern for the current platform.
	resolvedPattern := ExpandPattern(t.ResolvePattern(runtime.GOOS, runtime.GOARCH))

	// Download to cache
	cacheDir := m.Dirs.CacheDir(name)
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return fmt.Errorf("creating cache dir: %w", err)
	}

	args := []string{"release", "download", "-R", t.Repo, "-D", cacheDir, "--clobber"}
	if resolvedPattern != "" {
		args = append(args, "-p", resolvedPattern)
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

	// Write per-machine state. Only resolved/relevant fields are persisted;
	// the manifest remains the source of truth for the source spec.
	state := InstalledState{
		Repo:        t.Repo,
		Tag:         tag,
		Pattern:     resolvedPattern,
		Bin:         t.Bin,
		Man:         t.Man,
		Completions: t.Completions,
		InstalledAt: time.Now().UTC().Format(time.RFC3339),
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

	m.cleanupInstall(name)

	fmt.Printf("✓ Removed %s\n", name)
	return nil
}

// CleanupInstall removes all on-disk artifacts for a tool without touching
// the manifest: symlinks pointing into the tool's ToolDir, the ToolDir
// itself, its cache directory, and the state file. Safe to call when no
// prior install exists.
func (m *Manager) CleanupInstall(name string) {
	m.cleanupInstall(name)
}

// cleanupInstall removes all on-disk artifacts for a tool: symlinks pointing
// into the tool's ToolDir, the ToolDir itself, its cache directory, and the
// state file. The manifest entry is not touched.
func (m *Manager) cleanupInstall(name string) {
	m.removeToolSymlinks(name)
	_ = os.RemoveAll(m.Dirs.ToolDir(name))
	_ = os.RemoveAll(m.Dirs.CacheDir(name))
	_ = os.Remove(m.Dirs.StateFile(name))
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

// ListInstalled returns all installed tools by scanning the state directory.
// Results are sorted by tool name.
func (m *Manager) ListInstalled() ([]InstalledState, error) {
	entries, err := os.ReadDir(m.Dirs.State)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var states []InstalledState
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".toml") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".toml")
		s := m.ReadState(name)
		if s == nil || s.Repo == "" {
			continue
		}
		states = append(states, *s)
	}
	sort.Slice(states, func(i, j int) bool {
		return states[i].Repo < states[j].Repo
	})
	return states, nil
}

// BackfillState fills in missing fields on a state record from a manifest
// entry. This handles state files written before the schema was expanded.
// Mutates state in place.
func BackfillState(state *InstalledState, manifestTool *config.Tool) {
	if state == nil || manifestTool == nil {
		return
	}
	if len(state.Bin) == 0 {
		state.Bin = manifestTool.Bin
	}
	if len(state.Man) == 0 {
		state.Man = manifestTool.Man
	}
	if len(state.Completions) == 0 {
		state.Completions = manifestTool.Completions
	}
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
		bin = ExpandPattern(bin)
		srcName, linkName := parseBinSpec(bin)
		src := findFileInDir(toolDir, srcName)
		if src == "" {
			return fmt.Errorf("binary %q not found in extracted files", srcName)
		}
		dst := filepath.Join(m.Dirs.BinDir(), linkName)
		if err := forceSymlink(src, dst); err != nil {
			return fmt.Errorf("symlink %s: %w", linkName, err)
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

func (m *Manager) removeToolSymlinks(name string) {
	toolDir := m.Dirs.ToolDir(name)
	dirs := []string{
		m.Dirs.BinDir(),
		m.Dirs.ManDir(),
		m.Dirs.ZshCompletionDir(),
		m.Dirs.BashCompletionDir(),
	}
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			path := filepath.Join(dir, e.Name())
			info, err := os.Lstat(path)
			if err != nil || info.Mode()&os.ModeSymlink == 0 {
				continue
			}
			target, err := os.Readlink(path)
			if err != nil {
				continue
			}
			if !filepath.IsAbs(target) {
				target = filepath.Join(dir, target)
			}
			resolved, err := filepath.Abs(target)
			if err != nil {
				continue
			}
			absToolDir, err := filepath.Abs(toolDir)
			if err != nil {
				continue
			}
			if pathWithin(resolved, absToolDir) {
				_ = os.Remove(path)
			}
		}
	}
}

// pathWithin reports whether child is equal to or nested inside parent.
func pathWithin(child, parent string) bool {
	parent = filepath.Clean(parent)
	child = filepath.Clean(child)
	if child == parent {
		return true
	}
	sep := string(filepath.Separator)
	return strings.HasPrefix(child, parent+sep)
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

// ExpandPattern replaces template placeholders in a pattern with runtime-detected values.
//
// Supported variables:
//
//	{{os}}       — Go-style OS name: darwin, linux, windows
//	{{arch}}     — Go-style arch: arm64, amd64, i386
//	{{triple}}   — Rust target triple: aarch64-apple-darwin, x86_64-unknown-linux-gnu, …
//	{{platform}} — User-facing OS name: macos, linux, windows
//	{{gnuarch}}  — GNU/Rust-style arch: aarch64, x86_64, i686
func ExpandPattern(pattern string) string {
	os := normalizeOS(runtime.GOOS)
	arch := normalizeArch(runtime.GOARCH)
	triple := platformTriple(runtime.GOOS, runtime.GOARCH)
	platform := platformName(runtime.GOOS)
	gnu := gnuArch(runtime.GOARCH)
	pattern = strings.ReplaceAll(pattern, "{{os}}", os)
	pattern = strings.ReplaceAll(pattern, "{{arch}}", arch)
	pattern = strings.ReplaceAll(pattern, "{{triple}}", triple)
	pattern = strings.ReplaceAll(pattern, "{{platform}}", platform)
	pattern = strings.ReplaceAll(pattern, "{{gnuarch}}", gnu)
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

// platformName returns the user-facing platform name for a GOOS value.
// Examples: darwin → macos, linux → linux, windows → windows
func platformName(goos string) string {
	switch goos {
	case "darwin":
		return "macos"
	default:
		return goos
	}
}

// gnuArch returns the GNU/Rust-style architecture name for a GOARCH value.
// Examples: arm64 → aarch64, amd64 → x86_64, 386 → i686
func gnuArch(goarch string) string {
	switch goarch {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "aarch64"
	case "386":
		return "i686"
	default:
		return goarch
	}
}

// platformTriple returns a Rust-style target triple for the current platform.
// Examples: x86_64-unknown-linux-gnu, aarch64-apple-darwin, x86_64-pc-windows-msvc
func platformTriple(goos, goarch string) string {
	arch := gnuArch(goarch)

	switch goos {
	case "darwin":
		return arch + "-apple-darwin"
	case "linux":
		return arch + "-unknown-linux-gnu"
	case "windows":
		return arch + "-pc-windows-msvc"
	default:
		return arch + "-" + goos
	}
}

// parseBinSpec parses a bin entry which may be "name" or "source:link".
// "name" means find and symlink as "name".
// "source:link" means find "source" in the extracted files and create a symlink named "link".
func parseBinSpec(spec string) (source, link string) {
	if idx := strings.Index(spec, ":"); idx > 0 && idx < len(spec)-1 {
		return spec[:idx], spec[idx+1:]
	}
	return spec, spec
}
