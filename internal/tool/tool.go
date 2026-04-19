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
	Dirs     paths.Dirs
	reporter Reporter
}

// NewManager creates a Manager with resolved XDG paths and a no-op reporter.
// Use SetReporter (or WithReporter) to attach a real reporter.
func NewManager(dirs paths.Dirs) *Manager {
	return &Manager{Dirs: dirs, reporter: nopReporter{}}
}

// SetReporter attaches a Reporter to receive progress events. Passing nil
// resets the manager to the no-op reporter.
func (m *Manager) SetReporter(r Reporter) {
	if r == nil {
		m.reporter = nopReporter{}
		return
	}
	m.reporter = r
}

// WithReporter is a chainable helper equivalent to SetReporter.
func (m *Manager) WithReporter(r Reporter) *Manager {
	m.SetReporter(r)
	return m
}

// Install downloads, extracts, and symlinks a tool. The given config.Tool
// should carry the *source* spec (raw Pattern/Patterns from the manifest or
// CLI flags); Install resolves the platform-specific pattern, expands template
// variables (including {{tag}} once the latest tag is known), and installs.
// If verify is true, attempts attestation verification on the downloaded asset.
func (m *Manager) Install(t config.Tool, verify bool) error {
	name := t.Name()
	m.reporter.Start(name)
	assetPath, tag, resolvedPattern, err := m.DownloadAsset(t)
	if err != nil {
		m.reporter.Fail(name, err)
		return err
	}
	if err := m.installFromAsset(t, assetPath, tag, resolvedPattern, verify); err != nil {
		m.reporter.Fail(name, err)
		return err
	}
	m.reporter.Done(name, tag)
	return nil
}

// DownloadAsset resolves the platform-specific pattern, resolves the latest
// tag (if not pinned), and downloads the matching release asset to the cache.
// The cache directory is cleared before download so subsequent calls (e.g.
// findDownloadedAsset) cannot pick up stale files. Returns the local path of
// the downloaded asset, the tag actually downloaded, and the resolved+
// expanded pattern used for the download.
func (m *Manager) DownloadAsset(t config.Tool) (assetPath, tag, resolvedPattern string, err error) {
	name := t.Name()

	if err := m.Dirs.EnsureDirs(); err != nil {
		return "", "", "", fmt.Errorf("creating directories: %w", err)
	}

	// Resolve tag first so {{tag}} works in patterns.
	tag = t.Tag
	if tag == "" || tag == "latest" {
		resolved, resolveErr := resolveLatestTag(t.Repo)
		if resolveErr != nil {
			return "", "", "", fmt.Errorf("resolving latest tag for %s: %w", t.Repo, resolveErr)
		}
		tag = resolved
	}

	rawPattern := t.ResolvePattern(runtime.GOOS, runtime.GOARCH)
	if rawPattern == "" {
		return "", "", "", fmt.Errorf("%s: no pattern for %s/%s (unsupported on this platform)", t.Repo, runtime.GOOS, runtime.GOARCH)
	}
	resolvedPattern = ExpandPattern(rawPattern, tag)

	cacheDir := m.Dirs.CacheDir(name)
	// Clear the cache dir so findDownloadedAsset cannot pick up a stale
	// asset from a previous install/inspection.
	_ = os.RemoveAll(cacheDir)
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", "", "", fmt.Errorf("creating cache dir: %w", err)
	}

	args := []string{"release", "download", tag, "-R", t.Repo, "-D", cacheDir, "-p", resolvedPattern, "--clobber"}

	m.reporter.Stage(name, fmt.Sprintf("Downloading %s %s", t.Repo, tag))
	if _, _, err := gh.Exec(args...); err != nil {
		return "", "", "", fmt.Errorf("downloading release: %w", err)
	}

	assetPath, err = findDownloadedAsset(cacheDir)
	if err != nil {
		return "", "", "", err
	}
	return assetPath, tag, resolvedPattern, nil
}

// InstallFromAsset installs a tool from an already-downloaded asset. Performs
// optional attestation verification, extracts the archive into the tool dir,
// creates symlinks, and writes the per-machine state file. The resolved
// pattern is recomputed for the host platform so callers without one in
// hand still work.
func (m *Manager) InstallFromAsset(t config.Tool, assetPath, tag string, verify bool) error {
	resolvedPattern := ExpandPattern(t.ResolvePattern(runtime.GOOS, runtime.GOARCH), tag)
	return m.installFromAsset(t, assetPath, tag, resolvedPattern, verify)
}

func (m *Manager) installFromAsset(t config.Tool, assetPath, tag, resolvedPattern string, verify bool) error {
	name := t.Name()

	if verify {
		m.verifyAttestation(name, t.Repo, assetPath)
	}

	// Clean previous install
	toolDir := m.Dirs.ToolDir(name)
	_ = os.RemoveAll(toolDir)

	m.reporter.Stage(name, fmt.Sprintf("Extracting %s", filepath.Base(assetPath)))
	if err := archive.Extract(assetPath, toolDir); err != nil {
		return fmt.Errorf("extracting: %w", err)
	}

	if err := m.createSymlinks(t, tag, toolDir); err != nil {
		return fmt.Errorf("creating symlinks: %w", err)
	}

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

	return nil
}

// Remove uninstalls a tool by removing symlinks, tool dir, cache, and state.
func (m *Manager) Remove(t config.Tool) error {
	name := t.Name()
	m.reporter.Start(name)
	m.cleanupInstall(name)
	m.reporter.Done(name, "")
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

func (m *Manager) createSymlinks(t config.Tool, tag, toolDir string) error {
	name := t.Name()
	bins := t.Bin
	if len(bins) == 0 {
		bins = []string{name}
	}

	// Bin symlinks
	for _, bin := range bins {
		bin = ExpandPattern(bin, tag)
		srcName, linkName := parseBinSpec(bin)
		src := findFileInDir(toolDir, srcName)
		if src == "" {
			return fmt.Errorf("binary %q not found in extracted files", srcName)
		}
		// Some upstream archives (e.g. tree-sitter/tree-sitter zips) ship
		// binaries without the execute bit set. Repair before symlinking
		// so the user can actually run the resulting bin.
		if info, err := os.Stat(src); err == nil && info.Mode().IsRegular() && info.Mode()&0o111 == 0 {
			_ = os.Chmod(src, info.Mode()|0o111)
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
			m.reporter.Warn(name, fmt.Sprintf("man page %q not found", man))
			continue
		}
		dst := filepath.Join(m.Dirs.ManDir(), filepath.Base(man))
		if err := forceSymlink(src, dst); err != nil {
			m.reporter.Warn(name, fmt.Sprintf("man page %q: %s", man, err))
		}
	}

	// Completion symlinks. Route by extension/prefix:
	//   *.fish        → fish completions dir
	//   *.ps1         → PowerShell completions dir
	//   _<name>       → zsh completions dir (already in autoload form)
	//   *.zsh         → zsh completions dir, renamed to _<name> for autoload
	//   *.bash        → bash completions dir
	//   anything else → bash completions dir
	for _, comp := range t.Completions {
		src := findFileInDir(toolDir, comp)
		if src == "" {
			m.reporter.Warn(name, fmt.Sprintf("completion %q not found", comp))
			continue
		}
		base := filepath.Base(comp)
		low := strings.ToLower(base)
		var dst string
		switch {
		case strings.HasSuffix(low, ".fish"):
			dst = filepath.Join(m.Dirs.FishCompletionDir(), base)
		case strings.HasSuffix(low, ".ps1"):
			dst = filepath.Join(m.Dirs.PwshCompletionDir(), base)
		case strings.HasPrefix(base, "_"):
			dst = filepath.Join(m.Dirs.ZshCompletionDir(), base)
		case strings.HasSuffix(low, ".zsh"):
			stem := strings.TrimSuffix(base, filepath.Ext(base))
			dst = filepath.Join(m.Dirs.ZshCompletionDir(), "_"+stem)
		case strings.HasSuffix(low, ".bash"):
			dst = filepath.Join(m.Dirs.BashCompletionDir(), base)
		default:
			dst = filepath.Join(m.Dirs.BashCompletionDir(), base)
		}
		if err := forceSymlink(src, dst); err != nil {
			m.reporter.Warn(name, fmt.Sprintf("completion %q: %s", comp, err))
		}
	}

	return nil
}

func (m *Manager) removeToolSymlinks(name string) {
	toolDir := m.Dirs.ToolDir(name)
	for _, dir := range m.Dirs.SymlinkDirs() {
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
// This is best-effort: it surfaces a warning via the reporter if verification
// fails but does not return an error. On success the next stage (Extracting)
// is the user's signal that verification passed.
func (m *Manager) verifyAttestation(name, repo, assetPath string) {
	m.reporter.Stage(name, fmt.Sprintf("Verifying attestation for %s", filepath.Base(assetPath)))
	if _, _, err := gh.Exec("attestation", "verify", assetPath, "-R", repo); err != nil {
		m.reporter.Stage(name, "attestation not verified (this is expected for most repos)")
	}
}

// Tokens returns the literal expansions of every supported template token
// for the given platform and tag. Used by ExpandPattern and by the discover
// package when reverse-folding asset names into patterns.
func Tokens(goos, goarch, tag string) map[string]string {
	return map[string]string{
		"{{os}}":         normalizeOS(goos),
		"{{arch}}":       normalizeArch(goarch),
		"{{triple}}":     platformTriple(goos, goarch),
		"{{musltriple}}": platformMuslTriple(goos, goarch),
		"{{platform}}":   platformName(goos),
		"{{gnuarch}}":    gnuArch(goarch),
		"{{tag}}":        tag,
	}
}

// ExpandPatternFor expands template variables for an arbitrary platform.
// ExpandPattern is the convenience wrapper for the host platform.
func ExpandPatternFor(pattern, tag, goos, goarch string) string {
	for token, value := range Tokens(goos, goarch, tag) {
		pattern = strings.ReplaceAll(pattern, token, value)
	}
	return pattern
}

// ExpandPattern replaces template placeholders in a pattern with runtime-detected values.
//
// Supported variables:
//
//	{{os}}         — Go-style OS name: darwin, linux, windows
//	{{arch}}       — Go-style arch: arm64, amd64, i386
//	{{triple}}     — Rust target triple (Tier 1 Linux uses gnu)
//	{{musltriple}} — Rust target triple, Linux uses musl libc
//	{{platform}}   — User-facing OS name: macos, linux, windows
//	{{gnuarch}}    — GNU/Rust-style arch: aarch64, x86_64, i686
//	{{tag}}        — release tag, e.g. v1.2.3 (empty if not yet resolved)
func ExpandPattern(pattern, tag string) string {
	return ExpandPatternFor(pattern, tag, runtime.GOOS, runtime.GOARCH)
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

// platformMuslTriple is like platformTriple but uses musl as the libc on
// Linux. Many Rust CLIs (uv, ruff, just, watchexec, …) ship statically
// linked musl artifacts as their preferred Linux distribution.
func platformMuslTriple(goos, goarch string) string {
	if goos == "linux" {
		return gnuArch(goarch) + "-unknown-linux-musl"
	}
	return platformTriple(goos, goarch)
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
