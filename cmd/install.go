package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/ascarter/gh-tool/internal/config"
	"github.com/ascarter/gh-tool/internal/tool"
)

var installCmd = &cobra.Command{
	Use:   "install [owner/repo]",
	Short: "Install a tool from a GitHub release",
	Long: `Install a tool from a GitHub release. Downloads the release asset,
extracts it, and creates symlinks for binaries, man pages, and completions.

With no arguments, installs all tools defined in the manifest.`,
	RunE: runInstall,
}

var (
	flagPattern  string
	flagTag      string
	flagBin      []string
	flagMan      []string
	flagComp     []string
	flagNoVerify bool
)

func init() {
	installCmd.Flags().StringVarP(&flagPattern, "pattern", "p", "", "glob pattern to match release asset (supports {{os}} and {{arch}})")
	installCmd.Flags().StringVarP(&flagTag, "tag", "t", "", "release tag (default: latest)")
	installCmd.Flags().StringSliceVar(&flagBin, "bin", nil, "binary name(s) to symlink")
	installCmd.Flags().StringSliceVar(&flagMan, "man", nil, "man page path(s) relative to extracted archive")
	installCmd.Flags().StringSliceVar(&flagComp, "completion", nil, "completion file path(s) relative to extracted archive")
	installCmd.Flags().BoolVar(&flagNoVerify, "no-verify", false, "skip attestation verification")
}

func runInstall(cmd *cobra.Command, args []string) error {
	dirs := resolveDirs()
	mgr := tool.NewManager(dirs)

	cfg, err := config.Load(dirs.ConfigFile())
	if err != nil {
		return err
	}

	// No args: install everything in the manifest
	if len(args) == 0 {
		if len(cfg.Tools) == 0 {
			fmt.Println("No tools in manifest. Use: gh tool install <owner/repo> --pattern <pattern>")
			return nil
		}
		verify := !flagNoVerify
		for _, t := range cfg.Tools {
			t.Pattern = tool.ExpandPattern(t.Pattern)
			if err := mgr.Install(t, verify); err != nil {
				fmt.Fprintf(os.Stderr, "✗ %s: %s\n", t.Repo, err)
			}
		}
		return nil
	}

	repo := args[0]

	// Build tool config from flags, merging with existing manifest entry
	t := config.Tool{Repo: repo}
	if existing := cfg.FindTool(repo); existing != nil {
		t = *existing
	}

	// CLI flags override manifest values
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

	if t.Pattern == "" {
		return fmt.Errorf("--pattern is required (which release asset to download)")
	}

	// Expand {{os}}/{{arch}} template variables
	t.Pattern = tool.ExpandPattern(t.Pattern)

	// Install the tool
	if err := mgr.Install(t, !flagNoVerify); err != nil {
		return err
	}

	// Update manifest
	cfg.AddOrUpdateTool(t)
	return config.Save(dirs.ConfigFile(), cfg)
}
