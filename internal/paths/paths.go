package paths

import (
	"fmt"
	"os"
	"path/filepath"
)

const appName = "gh-tool"

// Dirs holds resolved directory paths for gh-tool.
type Dirs struct {
	Config string
	Data   string
	State  string
	Cache  string
}

// Resolve returns Dirs using the following precedence:
//  1. $GHTOOL_HOME — single-root override (config/data/state/cache live as
//     subdirectories of the prefix; the prefix itself names the app, so no
//     extra "gh-tool" segment is appended).
//  2. XDG Base Directory env vars ($XDG_CONFIG_HOME etc.), each with a
//     "gh-tool" segment appended.
//  3. Platform defaults (~/.config, ~/.local/share, ~/.local/state, ~/.cache),
//     each with a "gh-tool" segment appended.
func Resolve() Dirs {
	if home := os.Getenv("GHTOOL_HOME"); home != "" {
		return Dirs{
			Config: filepath.Join(home, "config"),
			Data:   filepath.Join(home, "data"),
			State:  filepath.Join(home, "state"),
			Cache:  filepath.Join(home, "cache"),
		}
	}
	return Dirs{
		Config: filepath.Join(xdgDir("XDG_CONFIG_HOME", defaultConfigHome()), appName),
		Data:   filepath.Join(xdgDir("XDG_DATA_HOME", defaultDataHome()), appName),
		State:  filepath.Join(xdgDir("XDG_STATE_HOME", defaultStateHome()), appName),
		Cache:  filepath.Join(xdgDir("XDG_CACHE_HOME", defaultCacheHome()), appName),
	}
}

// BinDir returns the path where tool binary symlinks are created.
func (d Dirs) BinDir() string {
	return filepath.Join(d.Data, "bin")
}

// ManDir returns the path where man page symlinks are created.
func (d Dirs) ManDir() string {
	return filepath.Join(d.Data, "share", "man", "man1")
}

// ZshCompletionDir returns the path for zsh completion symlinks.
func (d Dirs) ZshCompletionDir() string {
	return filepath.Join(d.Data, "share", "completions", "zsh")
}

// BashCompletionDir returns the path for bash completion symlinks.
func (d Dirs) BashCompletionDir() string {
	return filepath.Join(d.Data, "share", "completions", "bash")
}

// FishCompletionDir returns the path for fish completion symlinks.
func (d Dirs) FishCompletionDir() string {
	return filepath.Join(d.Data, "share", "completions", "fish")
}

// PwshCompletionDir returns the path for PowerShell completion symlinks.
func (d Dirs) PwshCompletionDir() string {
	return filepath.Join(d.Data, "share", "completions", "pwsh")
}

// ToolsDir returns the base path where tool payloads are unpacked.
func (d Dirs) ToolsDir() string {
	return filepath.Join(d.Data, "tools")
}

// ToolDir returns the path for a specific tool's unpacked payload.
func (d Dirs) ToolDir(name string) string {
	return filepath.Join(d.ToolsDir(), name)
}

// StateFile returns the path for a tool's installed version state file.
func (d Dirs) StateFile(name string) string {
	return filepath.Join(d.State, name+".toml")
}

// CacheDir returns the cache directory for a specific tool.
func (d Dirs) CacheDir(name string) string {
	return filepath.Join(d.Cache, name)
}

// ConfigFile returns the path to the TOML manifest.
func (d Dirs) ConfigFile() string {
	return filepath.Join(d.Config, "config.toml")
}

// SymlinkDirs returns every directory into which gh-tool installs symlinks:
// the bin dir, the man dir, and one dir per supported completion shell.
func (d Dirs) SymlinkDirs() []string {
	return []string{
		d.BinDir(),
		d.ManDir(),
		d.ZshCompletionDir(),
		d.BashCompletionDir(),
		d.FishCompletionDir(),
		d.PwshCompletionDir(),
	}
}

// EnsureDirs creates all required directories if they don't exist.
func (d Dirs) EnsureDirs() error {
	dirs := append([]string{d.Config, d.ToolsDir(), d.State, d.Cache}, d.SymlinkDirs()...)
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("creating %s: %w", dir, err)
		}
	}
	return nil
}

func xdgDir(envVar, fallback string) string {
	if v := os.Getenv(envVar); v != "" {
		return v
	}
	return fallback
}

func homeDir() string {
	h, _ := os.UserHomeDir()
	return h
}

func defaultConfigHome() string { return filepath.Join(homeDir(), ".config") }
func defaultDataHome() string   { return filepath.Join(homeDir(), ".local", "share") }
func defaultStateHome() string  { return filepath.Join(homeDir(), ".local", "state") }
func defaultCacheHome() string  { return filepath.Join(homeDir(), ".cache") }
