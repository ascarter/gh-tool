package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ascarter/gh-tool/internal/paths"
)

func TestPathSafe(t *testing.T) {
	cases := []struct {
		name    string
		path    string
		wantErr string
	}{
		{"empty", "", "empty"},
		{"whitespace", "   ", "empty"},
		{"root", "/", "root-like"},
		{"dot", ".", "root-like"},
		{"no-segment", "/home/user/.local/share", "gh-tool segment"},
		{"nested-no-segment", "/tmp/some/random/dir", "gh-tool segment"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := pathSafe(tc.path)
			if err == nil {
				t.Fatalf("pathSafe(%q) = nil, want error containing %q", tc.path, tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("pathSafe(%q) = %v, want error containing %q", tc.path, err, tc.wantErr)
			}
		})
	}

	ok := []string{
		"/home/user/.local/share/gh-tool",
		"/tmp/.cache/gh-tool/foo",
		filepath.Join(t.TempDir(), "gh-tool"),
	}
	for _, p := range ok {
		if err := pathSafe(p); err != nil {
			t.Errorf("pathSafe(%q) rejected a valid path: %v", p, err)
		}
	}
}

func TestPathSafe_GhtoolHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GHTOOL_HOME", home)

	// Strict descendant of GHTOOL_HOME: accepted even without a "gh-tool" segment.
	for _, sub := range []string{"data", "state", "cache", "data/bin"} {
		p := filepath.Join(home, sub)
		if err := pathSafe(p); err != nil {
			t.Errorf("pathSafe(%q) under GHTOOL_HOME rejected: %v", p, err)
		}
	}

	// GHTOOL_HOME itself (not a strict descendant) is still rejected.
	if err := pathSafe(home); err == nil {
		t.Errorf("pathSafe(%q) should reject GHTOOL_HOME itself", home)
	}

	// Unrelated path is rejected even with GHTOOL_HOME set.
	if err := pathSafe("/etc"); err == nil {
		t.Errorf("pathSafe(/etc) should be rejected even with GHTOOL_HOME set")
	}
}

// TestRunReset_Cleanup exercises the cleanup path against a temp Dirs via
// XDG_* env overrides. It verifies data/state/cache are wiped and the
// config file under Config is preserved. No network.
func TestRunReset_Cleanup(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))

	dirs := paths.Resolve()
	if err := dirs.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}

	// Seed files in every wipe target and in the preserved config dir.
	seed := map[string]string{
		dirs.ConfigFile():                                   "manifest = true\n",
		filepath.Join(dirs.Data, "marker"):                  "data",
		filepath.Join(dirs.State, "foo.toml"):               "stub",
		filepath.Join(dirs.Cache, "foo", "asset.tar.gz"):    "binary",
		filepath.Join(dirs.ToolsDir(), "foo", "bin", "foo"): "binary",
	}
	for p, content := range seed {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("MkdirAll %s: %v", filepath.Dir(p), err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", p, err)
		}
	}

	// Run the wipe directly (skip the manager/reporter loop: ListInstalled
	// will find zero tools because we only seeded arbitrary marker files).
	flagResetYes = true
	t.Cleanup(func() { flagResetYes = false })

	if err := runReset(resetCmd, nil); err != nil {
		t.Fatalf("runReset: %v", err)
	}

	// Wipe targets should be gone.
	for _, p := range []string{dirs.Data, dirs.State, dirs.Cache} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("expected %s removed, stat err = %v", p, err)
		}
	}
	// Config preserved.
	if _, err := os.Stat(dirs.ConfigFile()); err != nil {
		t.Errorf("expected config preserved, stat err = %v", err)
	}
}
