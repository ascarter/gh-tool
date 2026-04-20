package paths

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveDefaults(t *testing.T) {
	// Unset GHTOOL_HOME and XDG vars to test defaults
	t.Setenv("GHTOOL_HOME", "")
	for _, env := range []string{"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME"} {
		t.Setenv(env, "")
	}

	dirs := Resolve()
	home, _ := os.UserHomeDir()

	if !strings.HasPrefix(dirs.Config, home) {
		t.Errorf("Config = %q, should start with %q", dirs.Config, home)
	}
	if !strings.HasSuffix(dirs.Config, "gh-tool") {
		t.Errorf("Config = %q, should end with gh-tool", dirs.Config)
	}
}

func TestResolveWithEnv(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("GHTOOL_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(tmp, "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(tmp, "cache"))

	dirs := Resolve()

	if dirs.Config != filepath.Join(tmp, "config", "gh-tool") {
		t.Errorf("Config = %q", dirs.Config)
	}
	if dirs.Data != filepath.Join(tmp, "data", "gh-tool") {
		t.Errorf("Data = %q", dirs.Data)
	}
	if dirs.State != filepath.Join(tmp, "state", "gh-tool") {
		t.Errorf("State = %q", dirs.State)
	}
	if dirs.Cache != filepath.Join(tmp, "cache", "gh-tool") {
		t.Errorf("Cache = %q", dirs.Cache)
	}
}

func TestResolveGhtoolHome(t *testing.T) {
	tmp := t.TempDir()
	// XDG vars set but GHTOOL_HOME should win.
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "xdg-config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "xdg-data"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(tmp, "xdg-state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(tmp, "xdg-cache"))
	t.Setenv("GHTOOL_HOME", filepath.Join(tmp, "gh"))

	dirs := Resolve()

	want := Dirs{
		Config: filepath.Join(tmp, "gh", "config"),
		Data:   filepath.Join(tmp, "gh", "data"),
		State:  filepath.Join(tmp, "gh", "state"),
		Cache:  filepath.Join(tmp, "gh", "cache"),
	}
	if dirs != want {
		t.Errorf("Resolve() = %+v, want %+v", dirs, want)
	}

	// Sanity: helpers compose under the prefix and have no extra "gh-tool" segment.
	wantBin := filepath.Join(tmp, "gh", "data", "bin")
	if dirs.BinDir() != wantBin {
		t.Errorf("BinDir() = %q, want %q", dirs.BinDir(), wantBin)
	}
	if strings.Contains(dirs.Config, "gh-tool") {
		t.Errorf("Config = %q should not contain extra 'gh-tool' segment under GHTOOL_HOME", dirs.Config)
	}
}

func TestResolveGhtoolHomeEmptyFallsBackToXDG(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("GHTOOL_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(tmp, "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(tmp, "cache"))

	dirs := Resolve()
	if dirs.Config != filepath.Join(tmp, "config", "gh-tool") {
		t.Errorf("Config = %q, expected XDG layout", dirs.Config)
	}
}

func TestDirHelpers(t *testing.T) {
	dirs := Dirs{
		Config: "/config/gh-tool",
		Data:   "/data/gh-tool",
		State:  "/state/gh-tool",
		Cache:  "/cache/gh-tool",
	}

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"BinDir", dirs.BinDir(), "/data/gh-tool/bin"},
		{"ManDir", dirs.ManDir(), "/data/gh-tool/share/man/man1"},
		{"ZshCompletionDir", dirs.ZshCompletionDir(), "/data/gh-tool/share/completions/zsh"},
		{"BashCompletionDir", dirs.BashCompletionDir(), "/data/gh-tool/share/completions/bash"},
		{"ToolsDir", dirs.ToolsDir(), "/data/gh-tool/tools"},
		{"ToolDir(fzf)", dirs.ToolDir("fzf"), "/data/gh-tool/tools/fzf"},
		{"StateFile(fzf)", dirs.StateFile("fzf"), "/state/gh-tool/fzf.toml"},
		{"CacheDir(fzf)", dirs.CacheDir("fzf"), "/cache/gh-tool/fzf"},
		{"ConfigFile", dirs.ConfigFile(), "/config/gh-tool/config.toml"},
	}

	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.want)
		}
	}
}

func TestEnsureDirs(t *testing.T) {
	tmp := t.TempDir()
	dirs := Dirs{
		Config: filepath.Join(tmp, "config", "gh-tool"),
		Data:   filepath.Join(tmp, "data", "gh-tool"),
		State:  filepath.Join(tmp, "state", "gh-tool"),
		Cache:  filepath.Join(tmp, "cache", "gh-tool"),
	}

	if err := dirs.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}

	// Verify directories exist
	for _, dir := range []string{
		dirs.Config,
		dirs.BinDir(),
		dirs.ManDir(),
		dirs.ZshCompletionDir(),
		dirs.BashCompletionDir(),
		dirs.ToolsDir(),
		dirs.State,
		dirs.Cache,
	} {
		if _, err := os.Stat(dir); err != nil {
			t.Errorf("directory not created: %s", dir)
		}
	}
}
