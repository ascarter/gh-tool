package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ascarter/gh-tool/internal/config"
	"github.com/ascarter/gh-tool/internal/tool"
	"github.com/ascarter/gh-tool/internal/ui"
)

var removeCmd = &cobra.Command{
	Use:     "remove <owner/repo>",
	Aliases: []string{"rm", "uninstall"},
	Short:   "Remove an installed tool",
	Long: `Remove an installed tool's symlinks, payload, cache, and state.

The manifest is not modified. If the removed tool is still listed in the
manifest, a hint is printed reminding you that 'gh tool install' would
reinstall it.`,
	Args: cobra.ExactArgs(1),
	RunE: runRemove,
}

func runRemove(cmd *cobra.Command, args []string) error {
	repo := args[0]
	dirs := resolveDirs()
	mgr := tool.NewManager(dirs).WithReporter(removeReporter{})

	// Resolve tool name. Prefer state (handles repos installed via flags
	// without a manifest entry); fall back to the repo arg shape.
	t := config.Tool{Repo: repo}
	states, err := mgr.ListInstalled()
	if err != nil {
		return fmt.Errorf("listing installed tools: %w", err)
	}
	for _, s := range states {
		if s.Repo == repo {
			t = s.AsTool()
			break
		}
	}

	if err := mgr.Remove(t); err != nil {
		return err
	}

	// Best-effort manifest hint — never fail the command on this.
	mfPath := dirs.ConfigFile()
	if cfg, err := config.Load(mfPath); err == nil {
		if cfg.FindTool(repo) != nil {
			fmt.Printf("Note: %s is still listed in %s; running `gh tool install` would reinstall it.\n", repo, mfPath)
		}
	}

	return nil
}

// removeReporter prints just the success line for `remove`. The line
// reporter is overkill — there are no stages, no warnings to surface.
type removeReporter struct{}

func (removeReporter) Start(string)         {}
func (removeReporter) Stage(string, string) {}
func (removeReporter) Warn(string, string)  {}
func (removeReporter) Done(name, _ string)  { fmt.Printf("%s Removed %s\n", ui.Success(ui.IconSuccess), name) }
func (removeReporter) Fail(name string, err error) {
	fmt.Printf("%s %s: %s\n", ui.Error(ui.IconFailure), name, err)
}
