package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ascarter/gh-tool/internal/paths"
)

var shellCmd = &cobra.Command{
	Use:   "shell <bash|zsh|fish>",
	Short: "Generate shell integration config",
	Long: `Generate shell configuration for PATH, MANPATH, and completions.

Add to your shell profile:
  eval "$(gh tool shell bash)"
  eval "$(gh tool shell zsh)"
  gh tool shell fish | source`,
	Args: cobra.ExactArgs(1),
	RunE: runShell,
}

func init() {
	rootCmd.AddCommand(shellCmd)
}

func runShell(cmd *cobra.Command, args []string) error {
	return emitShell(resolveDirs(), args[0])
}

func emitShell(dirs paths.Dirs, shell string) error {
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

	case "fish":
		fmt.Printf(`# gh-tool shell integration (fish)
fish_add_path -g %s
set -gx MANPATH %s $MANPATH

# fish completions
if test -d %s
    for f in %s/*.fish
        source $f
    end
end
`, dirs.BinDir(), dirs.ManDir(), dirs.FishCompletionDir(), dirs.FishCompletionDir())

	default:
		return fmt.Errorf("unsupported shell: %s (use bash, zsh, or fish)", shell)
	}

	return nil
}

