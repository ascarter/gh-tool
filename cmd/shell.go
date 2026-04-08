package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var shellCmd = &cobra.Command{
	Use:   "shell <bash|zsh>",
	Short: "Generate shell integration config",
	Long: `Generate shell configuration for PATH, MANPATH, and completions.

Add to your shell profile:
  eval "$(gh tool shell bash)"
  eval "$(gh tool shell zsh)"`,
	Args: cobra.ExactArgs(1),
	RunE: runShell,
}

func init() {
	rootCmd.AddCommand(shellCmd)
}

func runShell(cmd *cobra.Command, args []string) error {
	dirs := resolveDirs()
	shell := args[0]

	switch shell {
	case "bash":
		fmt.Printf(`# gh-tool shell integration (bash)
export PATH="%s:$PATH"
export MANPATH="%s:$MANPATH"

# bash completions
if [[ -d "%s" ]]; then
  for f in "%s"/*; do
    [[ -f "$f" ]] && source "$f"
  done
fi
`, dirs.BinDir(), dirs.ManDir(), dirs.BashCompletionDir(), dirs.BashCompletionDir())

	case "zsh":
		fmt.Printf(`# gh-tool shell integration (zsh)
export PATH="%s:$PATH"
export MANPATH="%s:$MANPATH"

# zsh completions
fpath=(%s $fpath)
`, dirs.BinDir(), dirs.ManDir(), dirs.ZshCompletionDir())

	default:
		return fmt.Errorf("unsupported shell: %s (use bash or zsh)", shell)
	}

	return nil
}
