package cmd

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/ascarter/gh-tool/internal/tool"
)

var outdatedCmd = &cobra.Command{
	Use:   "outdated",
	Short: "List tools with upgrades available",
	Args:  cobra.NoArgs,
	RunE:  runOutdated,
}

func init() {
	rootCmd.AddCommand(outdatedCmd)
}

func runOutdated(cmd *cobra.Command, args []string) error {
	dirs := resolveDirs()
	mgr := tool.NewManager(dirs)

	states, err := mgr.ListInstalled()
	if err != nil {
		return err
	}
	if len(states) == 0 {
		return nil
	}

	stateByRepo := make(map[string]tool.InstalledState, len(states))
	repos := make([]string, 0, len(states))
	for _, s := range states {
		stateByRepo[s.Repo] = s
		repos = append(repos, s.Repo)
	}
	sort.Strings(repos)

	latestByRepo := fetchLatestTags(repos, stateByRepo)

	type outdatedRow struct {
		name, installed, latest string
	}
	var rows []outdatedRow
	for _, repo := range repos {
		s := stateByRepo[repo]
		latest := latestByRepo[repo]
		if latest == "" || latest == "?" {
			continue
		}
		if s.Tag == latest {
			continue
		}
		rows = append(rows, outdatedRow{
			name:      s.AsTool().Name(),
			installed: s.Tag,
			latest:    latest,
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].name < rows[j].name })

	if len(rows) == 0 {
		return nil
	}

	maxName := 0
	for _, r := range rows {
		if l := len(r.name); l > maxName {
			maxName = l
		}
	}
	for _, r := range rows {
		fmt.Printf("%-*s  (%s) < %s\n", maxName, r.name, r.installed, r.latest)
	}
	return nil
}
