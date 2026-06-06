package cmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/ascarter/gh-tool/internal/config"
	"github.com/ascarter/gh-tool/internal/paths"
	"github.com/ascarter/gh-tool/internal/tool"
)

func TestStringSlicesEqual(t *testing.T) {
	cases := []struct {
		a, b []string
		want bool
	}{
		{nil, nil, true},
		{nil, []string{}, true},
		{[]string{"x"}, []string{"x"}, true},
		{[]string{"a", "b"}, []string{"b", "a"}, true}, // unordered
		{[]string{"x"}, []string{"y"}, false},
		{[]string{"x", "y"}, []string{"x"}, false},
	}
	for _, c := range cases {
		if got := stringSlicesEqual(c.a, c.b); got != c.want {
			t.Errorf("stringSlicesEqual(%v, %v) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

// Regression: when a tool is already installed at the manifest's target
// tag but the manifest's bin/man/completions spec has changed (entries
// added or removed), isUpToDate must return false so install runs and
// actualizes the new spec. The previous implementation only compared
// the tag and would report "up to date" forever.
func TestIsUpToDateDetectsManifestSpecChange(t *testing.T) {
	root := t.TempDir()
	dirs := paths.Dirs{
		Config: filepath.Join(root, "config"),
		Data:   filepath.Join(root, "data"),
		State:  filepath.Join(root, "state"),
		Cache:  filepath.Join(root, "cache"),
	}
	if err := dirs.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}
	mgr := tool.NewManager(dirs)

	// Persist a state file as if the tool had been installed at tag v1
	// with bin=[fzf] only.
	state := tool.InstalledState{
		Repo:        "junegunn/fzf",
		Tag:         "v1",
		Pattern:     "fzf-v1.tar.gz",
		Bin:         []string{"fzf"},
		InstalledAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := writeStateForTest(dirs, "fzf", state); err != nil {
		t.Fatalf("write state: %v", err)
	}

	// Same tag, bin spec unchanged → up to date.
	tmatch := config.Tool{Repo: "junegunn/fzf", Tag: "v1", Bin: []string{"fzf"}}
	if !isUpToDate(mgr, tmatch) {
		t.Errorf("isUpToDate(matching spec)=false, want true")
	}

	// Same tag, manifest gained completions entry → must NOT be up to
	// date so install runs and creates the new completion symlinks.
	tadded := config.Tool{Repo: "junegunn/fzf", Tag: "v1", Bin: []string{"fzf"}, Completions: []string{"shell/completion.bash"}}
	if isUpToDate(mgr, tadded) {
		t.Errorf("isUpToDate(spec gained completions)=true, want false")
	}

	// Same tag, bin renamed → must NOT be up to date.
	trenamed := config.Tool{Repo: "junegunn/fzf", Tag: "v1", Bin: []string{"fzf-bin:fzf"}}
	if isUpToDate(mgr, trenamed) {
		t.Errorf("isUpToDate(renamed bin)=true, want false")
	}

	// Different tag → false regardless of spec. We pin Tag to bypass
	// the LatestTag network call.
	tnewtag := config.Tool{Repo: "junegunn/fzf", Tag: "v2", Bin: []string{"fzf"}}
	if isUpToDate(mgr, tnewtag) {
		t.Errorf("isUpToDate(different tag)=true, want false")
	}
}

// writeStateForTest seeds an InstalledState toml file at the path
// Manager.ReadState looks at. Avoids exporting Manager.writeState.
func writeStateForTest(dirs paths.Dirs, name string, state tool.InstalledState) error {
	path := dirs.StateFile(name)
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

