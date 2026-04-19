package cmd

import (
	"fmt"
	"os"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/ascarter/gh-tool/internal/config"
	"github.com/ascarter/gh-tool/internal/paths"
	"github.com/ascarter/gh-tool/internal/tool"
)

var installCmd = &cobra.Command{
	Use:   "install [owner/repo]",
	Short: "Install a tool from a GitHub release",
	Long: `Install a tool from a GitHub release. Downloads the release asset,
extracts it, and creates symlinks for binaries, man pages, and completions.

With no arguments, reconciles the local install set against the manifest:
installs anything missing, leaves up-to-date tools alone, and (with --force)
reinstalls everything.

The manifest is treated as a read-only input. To author a new manifest entry
interactively, use "gh tool add <owner/repo>".`,
	RunE: runInstall,
}

var (
	flagPattern  string
	flagTag      string
	flagBin      []string
	flagMan      []string
	flagComp     []string
	flagNoVerify bool
	flagForce    bool
	flagFile     string
)

func init() {
	installCmd.Flags().StringVarP(&flagPattern, "pattern", "p", "", "glob pattern to match release asset (supports {{os}} and {{arch}})")
	installCmd.Flags().StringVarP(&flagTag, "tag", "t", "", "release tag (default: latest)")
	installCmd.Flags().StringSliceVar(&flagBin, "bin", nil, "binary name(s) to symlink")
	installCmd.Flags().StringSliceVar(&flagMan, "man", nil, "man page path(s) relative to extracted archive")
	installCmd.Flags().StringSliceVar(&flagComp, "completion", nil, "completion file path(s) relative to extracted archive")
	installCmd.Flags().BoolVar(&flagNoVerify, "no-verify", false, "skip attestation verification")
	installCmd.Flags().BoolVar(&flagForce, "force", false, "reinstall even if up-to-date, clearing stale symlinks and cache")
	installCmd.Flags().StringVarP(&flagFile, "file", "f", "", "path to manifest file (default: $XDG_CONFIG_HOME/gh-tool/config.toml)")
}

// manifestPath returns the manifest path honoring --file, falling back to the
// XDG default.
func manifestPath(dirs paths.Dirs) string {
	if flagFile != "" {
		return flagFile
	}
	return dirs.ConfigFile()
}

func runInstall(cmd *cobra.Command, args []string) error {
	dirs := resolveDirs()
	mgr := tool.NewManager(dirs)
	mfPath := manifestPath(dirs)

	cfg, err := config.Load(mfPath)
	if err != nil {
		return err
	}

	if len(args) == 0 {
		if len(cfg.Tools) == 0 {
			fmt.Println("No tools in manifest. Use: gh tool add <owner/repo>")
			return nil
		}
		verify := !flagNoVerify
		for _, t := range cfg.Tools {
			if !t.ShouldInstallOn(runtime.GOOS) {
				fmt.Printf("· %s skipped on %s\n", t.Name(), runtime.GOOS)
				continue
			}
			if flagForce {
				mgr.CleanupInstall(t.Name())
			} else if isUpToDate(mgr, t) {
				continue
			}
			if err := mgr.Install(t, verify); err != nil {
				fmt.Fprintf(os.Stderr, "✗ %s: %s\n", t.Repo, err)
			}
		}
		return nil
	}

	repo := args[0]

	// Build tool config from flags, optionally seeded by manifest entry.
	t := config.Tool{Repo: repo}
	manifestEntry := cfg.FindTool(repo)

	if manifestEntry != nil {
		t = *manifestEntry
	}

	// CLI flags override manifest values.
	if flagPattern != "" {
		t.Pattern = flagPattern
	}
	if flagTag != "" {
		t.Tag = flagTag
	}
	if len(flagBin) > 0 {
		t.Bin = flagBin
	}
	if len(flagMan) > 0 {
		t.Man = flagMan
	}
	if len(flagComp) > 0 {
		t.Completions = flagComp
	}

	if t.Pattern == "" && len(t.Patterns) == 0 {
		if manifestEntry == nil {
			return fmt.Errorf("no manifest entry for %s; supply --pattern or run: gh tool add %s", repo, repo)
		}
		return fmt.Errorf("--pattern is required (which release asset to download)")
	}

	if flagForce {
		mgr.CleanupInstall(t.Name())
	} else if isUpToDate(mgr, t) {
		return nil
	}

	return mgr.Install(t, !flagNoVerify)
}

// isUpToDate checks whether a tool is already installed at the target version.
// Returns true (and prints a warning) if the installed version matches, false otherwise.
func isUpToDate(mgr *tool.Manager, t config.Tool) bool {
	name := t.Name()
	state := mgr.ReadState(name)
	if state == nil {
		return false
	}

	targetTag := t.Tag
	if targetTag == "" || targetTag == "latest" {
		latest, err := tool.LatestTag(t.Repo)
		if err != nil {
			return false
		}
		targetTag = latest
	}

	if state.Tag == targetTag {
		fmt.Printf("Warning: %s %s is already installed and up-to-date.\n", name, targetTag)
		return true
	}

	return false
}
