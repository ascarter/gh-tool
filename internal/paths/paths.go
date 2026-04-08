package paths

import (
	"os"
	"path/filepath"
	"runtime"
)

const appName = "gh-tool"

// Dirs holds resolved XDG-compliant directory paths for gh-tool.
type Dirs struct {
	Config string // $XDG_CONFIG_HOME/gh-tool
	Data   string // $XDG_DATA_HOME/gh-tool
	State  string // $XDG_STATE_HOME/gh-tool
	Cache  string // $XDG_CACHE_HOME/gh-tool
}

// Resolve returns Dirs with XDG paths resolved from environment variables
// with platform-appropriate defaults.
func Resolve() Dirs {
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

// EnsureDirs creates all required directories if they don't exist.
func (d Dirs) EnsureDirs() error {
	dirs := []string{
		d.Config,
		d.BinDir(),
		d.ManDir(),
		d.ZshCompletionDir(),
		d.BashCompletionDir(),
		d.ToolsDir(),
		d.State,
		d.Cache,
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
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

func defaultConfigHome() string {
	if runtime.GOOS == "darwin" {
		return filepath.Join(homeDir(), ".config")
	}
	return filepath.Join(homeDir(), ".config")
}

func defaultDataHome() string {
	if runtime.GOOS == "darwin" {
		return filepath.Join(homeDir(), ".local", "share")
	}
	return filepath.Join(homeDir(), ".local", "share")
}

func defaultStateHome() string {
	if runtime.GOOS == "darwin" {
		return filepath.Join(homeDir(), ".local", "state")
	}
	return filepath.Join(homeDir(), ".local", "state")
}

func defaultCacheHome() string {
	if runtime.GOOS == "darwin" {
		return filepath.Join(homeDir(), ".cache")
	}
	return filepath.Join(homeDir(), ".cache")
}
