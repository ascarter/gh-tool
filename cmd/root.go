package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ascarter/gh-tool/internal/paths"
)

var version = "dev"

var rootCmd = &cobra.Command{
	Use:   "gh-tool",
	Short: "Install and manage binary tools from GitHub releases.",
	Long:  "Install and manage binary tools from GitHub releases.",
}

func Execute() error {
	rootCmd.InitDefaultCompletionCmd()
	for _, c := range rootCmd.Commands() {
		if c.Name() == "completion" {
			c.Short = "Generate shell completion script"
			break
		}
	}
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(removeCmd)
	rootCmd.AddCommand(listCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("gh-tool %s\n", version)
	},
}

func resolveDirs() paths.Dirs {
	return paths.Resolve()
}
