package cmd

import (
	"fmt"
	"os"
	"runtime"
	"sync"

	"github.com/spf13/cobra"

	"github.com/ascarter/gh-tool/internal/config"
	"github.com/ascarter/gh-tool/internal/tool"
	"github.com/ascarter/gh-tool/internal/ui"
)

var (
	flagUpgradeJobs       int
	flagUpgradeNoProgress bool
	flagUpgradeVerbose    bool
)

var upgradeCmd = &cobra.Command{
	Use:   "upgrade [owner/repo]",
	Short: "Upgrade installed tools to the latest release",
	Long: `Upgrade installed tools to the latest release.

Drives off the local install state under $XDG_STATE_HOME/gh-tool/. The
manifest is not consulted.

With no arguments, upgrades all installed tools in parallel.
With an argument, upgrades only the specified tool.`,
	RunE: runUpgrade,
}

func init() {
	upgradeCmd.Flags().IntVarP(&flagUpgradeJobs, "jobs", "j", 0, "number of parallel upgrades (default: min(8, NumCPU))")
	upgradeCmd.Flags().BoolVar(&flagUpgradeNoProgress, "no-progress", false, "disable the live progress UI; print one line per event")
	upgradeCmd.Flags().BoolVarP(&flagUpgradeVerbose, "verbose", "v", false, "log every step (download, verify, extract); default shows only the result line per tool")
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

	// Resolve eligibility + latest tag in parallel: this is the slow part
	// of upgrade (one `gh release view` per tool). We then build the
	// install pool from only the tools that actually need an upgrade.
	type candidate struct {
		t      config.Tool
		latest string
	}
	candidates := make([]candidate, 0, len(states))
	var mu sync.Mutex

	checkJobs := make([]ui.Job, 0, len(states))
	for _, s := range states {
		s := s
		if cfg != nil {
			tool.BackfillState(&s, cfg.FindTool(s.Repo))
		}
		t := s.AsTool()
		name := t.Name()

		if !t.ShouldInstallOn(runtime.GOOS) {
			fmt.Printf("%s %s skipped on %s\n", ui.IconBullet, name, runtime.GOOS)
			continue
		}

		checkJobs = append(checkJobs, ui.Job{
			Name: name,
			Run: func() error {
				latest, err := tool.LatestTag(t.Repo)
				if err != nil {
					fmt.Fprintf(os.Stderr, "%s %s: could not check latest release: %s\n", ui.Error(ui.IconFailure), t.Repo, err)
					return nil // don't abort the batch
				}
				if s.Tag == latest {
					fmt.Printf("%s %s up to date (%s)\n", ui.IconBullet, name, latest)
					return nil
				}
				mu.Lock()
				candidates = append(candidates, candidate{t: t, latest: latest})
				mu.Unlock()
				return nil
			},
		})
	}

	if len(checkJobs) > 0 {
		_, _ = ui.Run(checkJobs, ui.ResolveJobs(flagUpgradeJobs))
	}

	if len(candidates) == 0 {
		return nil
	}

	useLive := !flagUpgradeNoProgress && ui.IsTTY() && len(candidates) > 1
	var live *ui.LiveReporter
	if useLive {
		live = ui.NewLiveReporter()
		_ = live.Launch()
		mgr.SetReporter(live)
		defer live.Stop()
	} else {
		mgr.SetReporter(ui.NewLineReporter(len(candidates) > 1, flagUpgradeVerbose))
	}

	jobs := make([]ui.Job, 0, len(candidates))
	for _, c := range candidates {
		t := c.t
		// Force tag to latest by clearing it.
		t.Tag = ""
		jobs = append(jobs, ui.Job{
			Name: t.Name(),
			Run:  func() error { return mgr.Install(t, true) },
		})
	}

	results, batchErr := ui.Run(jobs, ui.ResolveJobs(flagUpgradeJobs))
	if useLive {
		live.Stop()
	}
	failed := 0
	for _, r := range results {
		if r.Err != nil {
			failed++
		}
	}
	if failed > 0 {
		fmt.Fprintf(os.Stderr, "%s %d of %d upgrades failed\n", ui.Error(ui.IconFailure), failed, len(jobs))
		return batchErr
	}
	return nil
}
