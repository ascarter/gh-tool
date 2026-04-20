package cmd

import (
	"fmt"
	"os"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/ascarter/gh-tool/internal/config"
	"github.com/ascarter/gh-tool/internal/paths"
	"github.com/ascarter/gh-tool/internal/tool"
	"github.com/ascarter/gh-tool/internal/ui"
)

var installCmd = &cobra.Command{
	Use:   "install [owner/repo]",
	Short: "Install a tool from a GitHub release",
	Long: `Install a tool from a GitHub release.

With no arguments, reconciles the local install set against the manifest.
With an argument, installs a single tool (use --pattern, --bin, etc. for
ad-hoc installs not in the manifest).`,
	RunE: runInstall,
}

var (
	flagPattern    string
	flagTag        string
	flagBin        []string
	flagMan        []string
	flagComp       []string
	flagNoVerify   bool
	flagForce      bool
	flagFile       string
	flagJobs       int
	flagNoProgress bool
	flagVerbose    bool
)

func init() {
	installCmd.Flags().StringVarP(&flagPattern, "pattern", "p", "", "release asset glob (supports {{os}} and {{arch}})")
	installCmd.Flags().StringVarP(&flagTag, "tag", "t", "", "release tag (default: latest)")
	installCmd.Flags().StringSliceVar(&flagBin, "bin", nil, "binary name(s) to symlink")
	installCmd.Flags().StringSliceVar(&flagMan, "man", nil, "man page path(s) in archive")
	installCmd.Flags().StringSliceVar(&flagComp, "completion", nil, "completion path(s) in archive")
	installCmd.Flags().BoolVar(&flagNoVerify, "no-verify", false, "skip attestation verification")
	installCmd.Flags().BoolVar(&flagForce, "force", false, "reinstall even if up-to-date")
	installCmd.Flags().StringVarP(&flagFile, "file", "f", "", "manifest path (default: $XDG_CONFIG_HOME/gh-tool/config.toml)")
	installCmd.Flags().IntVarP(&flagJobs, "jobs", "j", 0, "parallel installs (default: min(8, NumCPU))")
	installCmd.Flags().BoolVar(&flagNoProgress, "no-progress", false, "disable the live progress UI")
	installCmd.Flags().BoolVarP(&flagVerbose, "verbose", "v", false, "log every step (download, verify, extract)")
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
		return runInstallReconcile(mgr, cfg)
	}

	// Single-tool install: keep linear path with line reporter.
	mgr.SetReporter(ui.NewLineReporter(false, flagVerbose))

	repo := args[0]
	t := config.Tool{Repo: repo}
	manifestEntry := cfg.FindTool(repo)

	if manifestEntry != nil {
		t = *manifestEntry
	}

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

// runInstallReconcile reconciles the local install set against the manifest
// in parallel. Tools filtered out by ShouldInstallOn or already at the
// target version are short-circuited before the worker pool spawns.
func runInstallReconcile(mgr *tool.Manager, cfg *config.Config) error {
	if len(cfg.Tools) == 0 {
		fmt.Println("No tools in manifest. Use: gh tool add <owner/repo>")
		return nil
	}

	verify := !flagNoVerify
	type pending struct {
		t config.Tool
	}
	var queue []pending

	for _, t := range cfg.Tools {
		if !t.ShouldInstallOn(runtime.GOOS) {
			fmt.Printf("%s %s skipped on %s\n", ui.IconBullet, t.Name(), runtime.GOOS)
			continue
		}
		if flagForce {
			mgr.CleanupInstall(t.Name())
		} else if isUpToDate(mgr, t) {
			continue
		}
		queue = append(queue, pending{t: t})
	}

	if len(queue) == 0 {
		return nil
	}

	// Choose reporter: live UI on TTY (when not disabled and >1 job),
	// line reporter otherwise.
	useLive := !flagNoProgress && ui.IsTTY() && len(queue) > 1
	var live *ui.LiveReporter
	if useLive {
		live = ui.NewLiveReporter()
		_ = live.Launch()
		mgr.SetReporter(live)
		defer live.Stop()
	} else {
		mgr.SetReporter(ui.NewLineReporter(len(queue) > 1, flagVerbose))
	}

	jobs := make([]ui.Job, 0, len(queue))
	for _, p := range queue {
		t := p.t
		jobs = append(jobs, ui.Job{
			Name: t.Name(),
			Run:  func() error { return mgr.Install(t, verify) },
		})
	}

	results, batchErr := ui.Run(jobs, ui.ResolveJobs(flagJobs))
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
		fmt.Fprintf(os.Stderr, "%s %d of %d installs failed\n", ui.Error(ui.IconFailure), failed, len(jobs))
		return batchErr
	}
	return nil
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
		fmt.Printf("%s %s up to date (%s)\n", ui.IconBullet, name, targetTag)
		return true
	}

	return false
}
