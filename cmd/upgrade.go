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

With no arguments, upgrades all tools in the manifest.
With an argument, upgrades only the specified tool.`,
	RunE: runUpgrade,
}

func init() {
	rootCmd.AddCommand(upgradeCmd)
}

func runUpgrade(cmd *cobra.Command, args []string) error {
	dirs := resolveDirs()
	mgr := tool.NewManager(dirs)

	cfg, err := config.Load(dirs.ConfigFile())
	if err != nil {
		return err
	}

	var tools []config.Tool
	if len(args) > 0 {
		t := cfg.FindTool(args[0])
		if t == nil {
			return fmt.Errorf("tool %s not found in manifest", args[0])
		}
		tools = append(tools, *t)
	} else {
		tools = cfg.Tools
	}

	if len(tools) == 0 {
		fmt.Println("No tools in manifest.")
		return nil
	}

	for _, t := range tools {
		name := t.Name()

		if !t.ShouldInstallOn(runtime.GOOS) {
			fmt.Printf("· %s skipped on %s\n", name, runtime.GOOS)
			continue
		}

		state := mgr.ReadState(name)

		latest, err := tool.LatestTag(t.Repo)
		if err != nil {
			fmt.Fprintf(os.Stderr, "✗ %s: could not check latest release: %s\n", t.Repo, err)
			continue
		}

		if state != nil && state.Tag == latest {
			fmt.Printf("· %s already at %s\n", name, latest)
			continue
		}

		// Override tag to latest for the upgrade
		upgradeT := t
		upgradeT.Tag = ""
		upgradeT.Pattern = upgradeT.ResolvePattern(runtime.GOOS, runtime.GOARCH)
		upgradeT.Pattern = tool.ExpandPattern(upgradeT.Pattern)
		if err := mgr.Install(upgradeT, true); err != nil {
			fmt.Fprintf(os.Stderr, "✗ %s: %s\n", t.Repo, err)
		}
	}
	return nil
}
