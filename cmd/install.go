package cmd

import (
	"fmt"
	"os"
	"runtime"
	"sort"

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
	flagPattern            string
	flagTag                string
	flagBin                []string
	flagMan                []string
	flagComp               []string
	flagNoVerify           bool
	flagRequireAttestation bool
	flagForce              bool
	flagFile               string
	flagJobs               int
	flagNoProgress         bool
	flagVerbose            bool
)

func init() {
	installCmd.Flags().StringVarP(&flagPattern, "pattern", "p", "", "release asset glob (supports {{os}} and {{arch}})")
	installCmd.Flags().StringVarP(&flagTag, "tag", "t", "", "release tag (default: latest)")
	installCmd.Flags().StringSliceVar(&flagBin, "bin", nil, "binary name(s) to symlink")
	installCmd.Flags().StringSliceVar(&flagMan, "man", nil, "man page path(s) in archive")
	installCmd.Flags().StringSliceVar(&flagComp, "completion", nil, "completion path(s) in archive")
	installCmd.Flags().BoolVar(&flagNoVerify, "no-verify", false, "skip attestation verification")
	installCmd.Flags().BoolVar(&flagRequireAttestation, "require-attestation", false, "fail if an attestation exists but does not verify")
	installCmd.Flags().BoolVar(&flagForce, "force", false, "reinstall even if up-to-date")
	installCmd.Flags().StringVarP(&flagFile, "file", "f", "", "manifest path (default: $XDG_CONFIG_HOME/gh-tool/config.toml)")
	installCmd.Flags().IntVarP(&flagJobs, "jobs", "j", 0, "parallel installs (default: min(8, NumCPU))")
	installCmd.Flags().BoolVar(&flagNoProgress, "no-progress", false, "disable the live progress UI")
	installCmd.Flags().BoolVarP(&flagVerbose, "verbose", "v", false, "log every step (download, verify, extract)")
	rootCmd.AddCommand(installCmd)
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
	mgr.RequireAttestation = flagRequireAttestation
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
	} else if targetTag, err := resolveTargetTag(t); err == nil {
		if state := mgr.ReadState(t.Name()); state != nil && upToDate(state, t, targetTag) {
			fmt.Printf("%s %s up to date (%s)\n", ui.IconBullet, t.Name(), targetTag)
			return nil
		}
		// Thread the resolved tag through so Install does not resolve it
		// a second time.
		t.Tag = targetTag
	}

	return mgr.Install(t, !flagNoVerify)
}

// runInstallReconcile reconciles the local install set against the manifest
// in parallel. Tools filtered out by ShouldInstallOn or already at the
// target version are short-circuited before the worker pool spawns. The
// target tag for each eligible tool is resolved once, up front (in
// parallel), and threaded into Install so the latest tag is not resolved a
// second time during download.
func runInstallReconcile(mgr *tool.Manager, cfg *config.Config) error {
	if len(cfg.Tools) == 0 {
		fmt.Println("No tools in manifest. Use: gh tool add <owner/repo>")
		return nil
	}

	verify := !flagNoVerify

	// Phase 1: filter by OS before making any network calls.
	type item struct {
		t          config.Tool
		targetTag  string
		resolveErr error
	}
	items := make([]*item, 0, len(cfg.Tools))
	for _, t := range cfg.Tools {
		if !t.ShouldInstallOn(runtime.GOOS) {
			fmt.Printf("%s %s skipped on %s\n", ui.IconBullet, t.Name(), runtime.GOOS)
			continue
		}
		items = append(items, &item{t: t})
	}
	if len(items) == 0 {
		return nil
	}

	// Phase 2: resolve each eligible tool's target tag in parallel. This is
	// the slow part of reconcile (one `gh release view` per unpinned tool).
	// Resolving once here lets us both compare against installed state and
	// thread the resolved tag into Install, avoiding a second resolution.
	resolveJobs := make([]ui.Job, 0, len(items))
	for _, it := range items {
		it := it
		resolveJobs = append(resolveJobs, ui.Job{
			Name: it.t.Name(),
			Run: func() error {
				it.targetTag, it.resolveErr = resolveTargetTag(it.t)
				return nil
			},
		})
	}
	_, _ = ui.Run(resolveJobs, ui.ResolveJobs(flagJobs))

	// Phase 3: compare, print, and queue serially so output stays ordered.
	queue := make([]config.Tool, 0, len(items))
	for _, it := range items {
		name := it.t.Name()
		if flagForce {
			mgr.CleanupInstall(name)
		}
		if it.resolveErr != nil {
			// Resolution failed; queue with the original spec so Install
			// re-resolves and surfaces the error through its normal path.
			queue = append(queue, it.t)
			continue
		}
		if !flagForce {
			if state := mgr.ReadState(name); state != nil && upToDate(state, it.t, it.targetTag) {
				fmt.Printf("%s %s up to date (%s)\n", ui.IconBullet, name, it.targetTag)
				continue
			}
		}
		t := it.t
		t.Tag = it.targetTag
		queue = append(queue, t)
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
	for _, t := range queue {
		t := t
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

// resolveTargetTag returns the tag a tool should be installed at: the pinned
// tag when one is set, otherwise the repo's latest release tag resolved via
// the GitHub API. An empty latest tag is treated as an error so callers don't
// accidentally thread an empty tag (which would silently fall back to
// re-resolution downstream).
func resolveTargetTag(t config.Tool) (string, error) {
	if t.Tag != "" && t.Tag != "latest" {
		return t.Tag, nil
	}
	tag, err := tool.LatestTag(t.Repo)
	if err != nil {
		return "", err
	}
	if tag == "" {
		return "", fmt.Errorf("no latest release tag for %s", t.Repo)
	}
	return tag, nil
}

// upToDate reports whether the installed copy already matches the target tag
// AND the manifest's asset spec. The spec check covers the case where the
// user added or renamed a bin/man/completion entry after the tool was already
// installed at the current release tag — without it, `gh tool install` would
// print "up to date" and never create the new symlinks. This function is pure:
// it performs no I/O and prints nothing, so the resolution of targetTag is the
// caller's responsibility.
func upToDate(state *tool.InstalledState, t config.Tool, targetTag string) bool {
	if state == nil {
		return false
	}
	if state.Tag != targetTag {
		return false
	}
	return stringSlicesEqual(state.Bin, t.Bin) &&
		stringSlicesEqual(state.Man, t.Man) &&
		stringSlicesEqual(state.Completions, t.Completions)
}

// stringSlicesEqual compares two slices as unordered sets, treating
// nil and empty as equivalent. Used by isUpToDate to detect manifest
// asset-spec changes that require a reinstall regardless of whether
// the release tag moved.
func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	if len(a) == 0 {
		return true
	}
	ac := append([]string(nil), a...)
	bc := append([]string(nil), b...)
	sort.Strings(ac)
	sort.Strings(bc)
	for i := range ac {
		if ac[i] != bc[i] {
			return false
		}
	}
	return true
}
