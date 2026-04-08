package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ascarter/gh-tool/internal/config"
	"github.com/ascarter/gh-tool/internal/tool"
)

var removeCmd = &cobra.Command{
	Use:     "remove <owner/repo>",
	Aliases: []string{"rm", "uninstall"},
	Short:   "Remove an installed tool",
	Long:    "Remove an installed tool, its symlinks, cached downloads, and manifest entry.",
	Args:    cobra.ExactArgs(1),
	RunE:    runRemove,
}

func runRemove(cmd *cobra.Command, args []string) error {
	repo := args[0]
	dirs := resolveDirs()
	mgr := tool.NewManager(dirs)

	cfg, err := config.Load(dirs.ConfigFile())
	if err != nil {
		return err
	}

	t := cfg.FindTool(repo)
	if t == nil {
		// Allow removal even if not in manifest by constructing a minimal tool
		t = &config.Tool{Repo: repo}
	}

	if err := mgr.Remove(*t); err != nil {
		return err
	}

	// Remove from manifest
	if cfg.RemoveTool(repo) {
		if err := config.Save(dirs.ConfigFile(), cfg); err != nil {
			return fmt.Errorf("updating manifest: %w", err)
		}
	}

	return nil
}
