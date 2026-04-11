package cmd

import (
	"fmt"
	"os"
	"runtime"

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
	Long:    "List installed tools with their version and update availability.",
	RunE:    runList,
}

func runList(cmd *cobra.Command, args []string) error {
	dirs := resolveDirs()
	mgr := tool.NewManager(dirs)

	cfg, err := config.Load(dirs.ConfigFile())
	if err != nil {
		return err
	}

	if len(cfg.Tools) == 0 {
		fmt.Println("No tools installed.")
		return nil
	}

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

	for _, t := range cfg.Tools {
		name := t.Name()

		// If the tool is filtered to other OSes, show it as skipped
		if !t.ShouldInstallOn(runtime.GOOS) {
			tp.AddField(t.Repo)
			tp.AddField("-")
			tp.AddField("-")
			tp.AddField("skipped (os)")
			tp.EndRow()
			continue
		}

		state := mgr.ReadState(name)

		installed := "-"
		if state != nil {
			installed = state.Tag
		}

		latest, err := tool.LatestTag(t.Repo)
		if err != nil {
			latest = "?"
		}

		status := ""
		if state == nil {
			status = "not installed"
		} else if installed != latest && latest != "?" {
			status = "update available"
		} else {
			status = "up to date"
		}

		tp.AddField(t.Repo)
		tp.AddField(installed)
		tp.AddField(latest)
		tp.AddField(status)
		tp.EndRow()
	}

	return tp.Render()
}
