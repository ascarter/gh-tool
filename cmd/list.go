package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/cli/go-gh/v2/pkg/tableprinter"
	"github.com/cli/go-gh/v2/pkg/term"
	"github.com/spf13/cobra"

	"github.com/ascarter/gh-tool/internal/config"
	"github.com/ascarter/gh-tool/internal/tool"
	"github.com/ascarter/gh-tool/internal/ui"
)

var (
	flagListOutdated bool
	flagListPinned   bool
)

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List installed tools",
	RunE:    runList,
}

func init() {
	listCmd.Flags().BoolVar(&flagListOutdated, "outdated", false, "show only tools with a newer release available")
	listCmd.Flags().BoolVar(&flagListPinned, "pinned", false, "show only tools pinned to a specific tag")
}

func runList(cmd *cobra.Command, args []string) error {
	dirs := resolveDirs()
	mgr := tool.NewManager(dirs)

	cfg, err := config.Load(dirs.ConfigFile())
	if err != nil {
		return err
	}

	states, err := mgr.ListInstalled()
	if err != nil {
		return err
	}

	if len(states) == 0 && len(cfg.Tools) == 0 {
		fmt.Println("No tools installed.")
		return nil
	}

	sort.Slice(states, func(i, j int) bool { return states[i].Repo < states[j].Repo })

	repos := make([]string, 0, len(states))
	stateByRepo := make(map[string]tool.InstalledState, len(states))
	for _, s := range states {
		repos = append(repos, s.Repo)
		stateByRepo[s.Repo] = s
	}

	latestByRepo := fetchLatestTags(repos, stateByRepo)

	type listRow struct {
		repo     string
		version  string // display string: tag, optionally with update arrow
		outdated bool
		pinned   bool
	}
	rows := make([]listRow, 0, len(states))
	for _, repo := range repos {
		s := stateByRepo[repo]
		latest := latestByRepo[repo]
		if latest == "" {
			latest = "?"
		}
		pinned := false
		if m := cfg.FindTool(repo); m != nil && m.Tag != "" && m.Tag != "latest" {
			pinned = true
		}
		outdated := latest != "?" && latest != s.Tag

		version := s.Tag
		if pinned {
			version += " (pinned)"
		}
		if outdated {
			version += " " + ui.IconArrow + " " + latest
		}

		rows = append(rows, listRow{
			repo:     repo,
			version:  version,
			outdated: outdated,
			pinned:   pinned,
		})
	}

	if flagListOutdated || flagListPinned {
		filtered := rows[:0]
		for _, r := range rows {
			if flagListOutdated && !r.outdated {
				continue
			}
			if flagListPinned && !r.pinned {
				continue
			}
			filtered = append(filtered, r)
		}
		rows = filtered
		if len(rows) == 0 {
			return nil
		}
	}

	terminal := term.FromEnv()
	w, _, _ := terminal.Size()
	if w == 0 {
		w = 80
	}
	tp := tableprinter.New(os.Stdout, terminal.IsTerminalOutput(), w)

	maxTool := len("TOOL")
	maxVersion := len("VERSION")
	for _, r := range rows {
		if l := len(r.repo); l > maxTool {
			maxTool = l
		}
		if l := len(r.version); l > maxVersion {
			maxVersion = l
		}
	}

	// Three physical columns: icon (blank or ↑), TOOL, VERSION.
	// Keeping the icon in its own column avoids ANSI-width miscalculation.
	tp.AddField("")
	tp.AddField("TOOL")
	tp.AddField("VERSION")
	tp.EndRow()
	tp.AddField(" ")
	tp.AddField(strings.Repeat("-", maxTool))
	tp.AddField(strings.Repeat("-", maxVersion))
	tp.EndRow()

	for _, r := range rows {
		if r.outdated {
			tp.AddField("↑", tableprinter.WithColor(ui.Warn))
		} else {
			tp.AddField("")
		}
		tp.AddField(r.repo)
		if r.outdated {
			tp.AddField(r.version, tableprinter.WithColor(ui.Warn))
		} else {
			tp.AddField(r.version)
		}
		tp.EndRow()
	}

	return tp.Render()
}

// fetchLatestTags resolves LatestTag for every installed repo in parallel.
func fetchLatestTags(repos []string, stateByRepo map[string]tool.InstalledState) map[string]string {
	out := map[string]string{}
	var mu sync.Mutex

	jobs := make([]ui.Job, 0, len(repos))
	for _, repo := range repos {
		if _, ok := stateByRepo[repo]; !ok {
			continue
		}
		repo := repo
		jobs = append(jobs, ui.Job{
			Name: repo,
			Run: func() error {
				latest, err := tool.LatestTag(repo)
				mu.Lock()
				if err != nil {
					out[repo] = "?"
				} else {
					out[repo] = latest
				}
				mu.Unlock()
				return nil
			},
		})
	}
	if len(jobs) == 0 {
		return out
	}
	_, _ = ui.Run(jobs, ui.DefaultJobs())
	return out
}
