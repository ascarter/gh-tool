package cmd

import (
	"fmt"
	"os"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/ascarter/gh-tool/internal/config"
	"github.com/ascarter/gh-tool/internal/tool"
)

var upgradeCmd = &cobra.Command{
	Use:   "upgrade [owner/repo]",
	Short: "Upgrade installed tools to the latest release",
	Long: `Upgrade installed tools to the latest release.

Drives off the local install state under $XDG_STATE_HOME/gh-tool/. The
manifest is not consulted.

With no arguments, upgrades all installed tools.
With an argument, upgrades only the specified tool.`,
	RunE: runUpgrade,
}

func init() {
	rootCmd.AddCommand(upgradeCmd)
}

func runUpgrade(cmd *cobra.Command, args []string) error {
	dirs := resolveDirs()
	mgr := tool.NewManager(dirs)

	var states []tool.InstalledState
	all, err := mgr.ListInstalled()
	if err != nil {
		return fmt.Errorf("listing installed tools: %w", err)
	}
	if len(args) > 0 {
		repo := args[0]
		for _, s := range all {
			if s.Repo == repo {
				states = append(states, s)
				break
			}
		}
		if len(states) == 0 {
			return fmt.Errorf("tool %s is not installed", repo)
		}
	} else {
		states = all
	}

	if len(states) == 0 {
		fmt.Println("No tools installed.")
		return nil
	}

	// Best-effort manifest load for backfilling pre-migration state files.
	cfg, _ := config.Load(dirs.ConfigFile())

	for _, s := range states {
		if cfg != nil {
			tool.BackfillState(&s, cfg.FindTool(s.Repo))
		}
		t := s.AsTool()
		name := t.Name()

		if !t.ShouldInstallOn(runtime.GOOS) {
			fmt.Printf("· %s skipped on %s\n", name, runtime.GOOS)
			continue
		}

		latest, err := tool.LatestTag(t.Repo)
		if err != nil {
			fmt.Fprintf(os.Stderr, "✗ %s: could not check latest release: %s\n", t.Repo, err)
			continue
		}

		if s.Tag == latest {
			fmt.Printf("· %s already at %s\n", name, latest)
			continue
		}

		// Force tag to latest.
		t.Tag = ""
		if err := mgr.Install(t, true); err != nil {
			fmt.Fprintf(os.Stderr, "✗ %s: %s\n", t.Repo, err)
		}
	}
	return nil
}
