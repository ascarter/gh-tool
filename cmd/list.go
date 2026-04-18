package cmd

import (
	"fmt"
	"os"
	"reflect"
	"runtime"
	"sort"

	"github.com/cli/go-gh/v2/pkg/tableprinter"
	"github.com/cli/go-gh/v2/pkg/term"
	"github.com/spf13/cobra"

	"github.com/ascarter/gh-tool/internal/config"
	"github.com/ascarter/gh-tool/internal/tool"
)

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List installed tools",
	Long: `List installed tools with their version, the latest available release,
and a status that captures any drift between the local install and the
manifest.

Status values:
  up to date         installed, on latest release, matches manifest
  update available   installed, but a newer release exists
  drift              installed spec differs from manifest spec; run 'gh tool install --force'
  orphan             installed but not present in the manifest
  pending            in manifest but not installed
  skipped (os)       in manifest, filtered out by 'os' on this platform`,
	RunE: runList,
}

func runList(cmd *cobra.Command, args []string) error {
	dirs := resolveDirs()
	mgr := tool.NewManager(dirs)

	cfg, err := config.Load(dirs.ConfigFile())
	if err != nil {
		return err
	}

	states, err := mgr.ListInstalled()
	if err != nil {
		return err
	}

	if len(states) == 0 && len(cfg.Tools) == 0 {
		fmt.Println("No tools installed.")
		return nil
	}

	// Build lookup tables.
	stateByRepo := make(map[string]tool.InstalledState, len(states))
	for _, s := range states {
		stateByRepo[s.Repo] = s
	}
	manifestByRepo := make(map[string]config.Tool, len(cfg.Tools))
	for _, t := range cfg.Tools {
		manifestByRepo[t.Repo] = t
	}

	// Union of repos, sorted.
	repoSet := make(map[string]struct{})
	for r := range stateByRepo {
		repoSet[r] = struct{}{}
	}
	for r := range manifestByRepo {
		repoSet[r] = struct{}{}
	}
	repos := make([]string, 0, len(repoSet))
	for r := range repoSet {
		repos = append(repos, r)
	}
	sort.Strings(repos)

	terminal := term.FromEnv()
	w, _, _ := terminal.Size()
	if w == 0 {
		w = 80
	}
	tp := tableprinter.New(os.Stdout, terminal.IsTerminalOutput(), w)

	tp.AddField("REPO")
	tp.AddField("INSTALLED")
	tp.AddField("LATEST")
	tp.AddField("STATUS")
	tp.EndRow()

	for _, repo := range repos {
		state, installed := stateByRepo[repo]
		manifest, inManifest := manifestByRepo[repo]

		// Pending or skipped — not installed.
		if !installed {
			if !manifest.ShouldInstallOn(runtime.GOOS) {
				row(tp, repo, "-", "-", "skipped (os)")
				continue
			}
			row(tp, repo, "-", "-", "pending")
			continue
		}

		latest, err := tool.LatestTag(state.Repo)
		if err != nil {
			latest = "?"
		}

		status := classifyInstalled(state, manifest, inManifest, latest)
		row(tp, repo, state.Tag, latest, status)
	}

	return tp.Render()
}

func row(tp tableprinter.TablePrinter, repo, installed, latest, status string) {
	tp.AddField(repo)
	tp.AddField(installed)
	tp.AddField(latest)
	tp.AddField(status)
	tp.EndRow()
}

// classifyInstalled returns the STATUS column value for an installed tool.
func classifyInstalled(state tool.InstalledState, manifest config.Tool, inManifest bool, latest string) string {
	if !inManifest {
		return "orphan"
	}
	if specDriftsFromManifest(state, manifest) {
		return "drift"
	}
	if latest != "?" && state.Tag != latest {
		return "update available"
	}
	return "up to date"
}

// specDriftsFromManifest reports whether the installed state was produced by
// a manifest spec different from the current manifest entry for the same
// repo. Compares the manifest's resolved pattern (for this host) and the
// per-tool fields actually persisted in state.
func specDriftsFromManifest(state tool.InstalledState, manifest config.Tool) bool {
	manifestResolved := tool.ExpandPattern(manifest.ResolvePattern(runtime.GOOS, runtime.GOARCH))
	if state.Pattern != "" && manifestResolved != "" && state.Pattern != manifestResolved {
		return true
	}
	if !sliceEqualOrEmpty(state.Bin, manifest.Bin) {
		return true
	}
	if !sliceEqualOrEmpty(state.Man, manifest.Man) {
		return true
	}
	if !sliceEqualOrEmpty(state.Completions, manifest.Completions) {
		return true
	}
	return false
}

// sliceEqualOrEmpty returns true if the slices are equal, or if the state
// slice is empty (treated as "not recorded — no opinion") regardless of the
// manifest slice.
func sliceEqualOrEmpty(stateSlice, manifestSlice []string) bool {
	if len(stateSlice) == 0 {
		return true
	}
	return reflect.DeepEqual(stateSlice, manifestSlice)
}
