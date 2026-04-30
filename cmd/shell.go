package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/ascarter/gh-tool/internal/paths"
)

var shellNoCompletions bool

var shellCmd = &cobra.Command{
	Use:   "shell [bash|zsh|fish]",
	Short: "Generate shell integration config",
	Long: `Generate shell configuration for PATH, MANPATH, and completions.

If no shell is given, gh-tool detects it from $SHELL.

Add to your shell profile:
  eval "$(gh tool shell)"          # auto-detect
  eval "$(gh tool shell bash)"
  eval "$(gh tool shell zsh)"
  gh tool shell fish | source

If $GHTOOL_HOME is set when this command runs, it is exported in the emitted
script so subshells inherit the same root.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runShell,
}

func init() {
	shellCmd.Flags().BoolVar(&shellNoCompletions, "no-completions", false, "Omit completion sourcing from the emitted script")
	rootCmd.AddCommand(shellCmd)
}

func runShell(cmd *cobra.Command, args []string) error {
	shell := ""
	if len(args) == 1 {
		shell = args[0]
	} else {
		detected, err := detectShell(os.Getenv("SHELL"))
		if err != nil {
			return err
		}
		shell = detected
	}
	return emitShell(resolveDirs(), shell, shellOptions{
		NoCompletions: shellNoCompletions,
		GhtoolHome:    os.Getenv("GHTOOL_HOME"),
	})
}

// detectShell maps a $SHELL value (e.g. "/bin/zsh") to a supported shell name.
func detectShell(shellEnv string) (string, error) {
	if shellEnv == "" {
		return "", fmt.Errorf("cannot detect shell: $SHELL is unset; pass bash, zsh, or fish explicitly")
	}
	base := filepath.Base(shellEnv)
	switch base {
	case "bash", "zsh", "fish":
		return base, nil
	default:
		return "", fmt.Errorf("cannot detect shell from $SHELL=%q (basename %q); pass bash, zsh, or fish explicitly", shellEnv, base)
	}
}

type shellOptions struct {
	NoCompletions bool
	GhtoolHome    string
}

func emitShell(dirs paths.Dirs, shell string, opts shellOptions) error {
	switch shell {
	case "bash":
		fmt.Printf(`# gh-tool shell integration (bash)
export PATH="%s:$PATH"
export MANPATH="%s:$MANPATH"
`, dirs.BinDir(), dirs.ManDir())
		if opts.GhtoolHome != "" {
			fmt.Printf("export GHTOOL_HOME=%q\n", opts.GhtoolHome)
		}
		if !opts.NoCompletions {
			fmt.Printf(`
# bash completions (interactive shells only)
if [[ $- == *i* ]] && [[ -d "%s" ]]; then
  for f in "%s"/*; do
    [[ -f "$f" ]] && source "$f"
  done
fi
`, dirs.BashCompletionDir(), dirs.BashCompletionDir())
		}

	case "zsh":
		fmt.Printf(`# gh-tool shell integration (zsh)
export PATH="%s:$PATH"
export MANPATH="%s:$MANPATH"
`, dirs.BinDir(), dirs.ManDir())
		if opts.GhtoolHome != "" {
			fmt.Printf("export GHTOOL_HOME=%q\n", opts.GhtoolHome)
		}
		if !opts.NoCompletions {
			fmt.Printf(`
# zsh completions (interactive shells only)
if [[ -o interactive ]]; then
  fpath=(%s $fpath)
fi
`, dirs.ZshCompletionDir())
		}

	case "fish":
		fmt.Printf(`# gh-tool shell integration (fish)
fish_add_path -g %s
set -gx MANPATH %s $MANPATH
`, dirs.BinDir(), dirs.ManDir())
		if opts.GhtoolHome != "" {
			fmt.Printf("set -gx GHTOOL_HOME %q\n", opts.GhtoolHome)
		}
		if !opts.NoCompletions {
			fmt.Printf(`
# fish completions (interactive shells only)
if status is-interactive; and test -d %s
    for f in %s/*.fish
        source $f
    end
end
`, dirs.FishCompletionDir(), dirs.FishCompletionDir())
		}

	default:
		return fmt.Errorf("unsupported shell: %s (use bash, zsh, or fish)", shell)
	}

	return nil
}
