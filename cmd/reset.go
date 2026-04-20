package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ascarter/gh-tool/internal/tool"
	"github.com/ascarter/gh-tool/internal/ui"
)

// appSegment is the path segment every gh-tool-owned directory must contain
// before reset will delete it. Must stay in sync with internal/paths.appName.
const appSegment = "gh-tool"

var flagResetYes bool

var resetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Remove all installed tools and clear gh-tool data",
	Long: `Return gh-tool to a pristine state.

Reset removes every installed tool (symlinks, payloads, state, cache) and
then wipes gh-tool's data, state, and cache directories.

Your manifest at $XDG_CONFIG_HOME/gh-tool/config.toml is preserved so that
'gh tool install' can restore everything later.

Reset does not remove the gh-tool extension itself. If you also want the
extension gone, run:

    gh extension remove ascarter/gh-tool`,
	Args: cobra.NoArgs,
	RunE: runReset,
}

func init() {
	resetCmd.Flags().BoolVarP(&flagResetYes, "yes", "y", false, "skip the confirmation prompt")
	rootCmd.AddCommand(resetCmd)
}

func runReset(cmd *cobra.Command, _ []string) error {
	dirs := resolveDirs()

	// Safety: refuse if any wipe target doesn't look like ours.
	wipeTargets := []string{dirs.Data, dirs.State, dirs.Cache}
	for _, p := range wipeTargets {
		if err := pathSafe(p); err != nil {
			return fmt.Errorf("refusing to remove %q: %w", p, err)
		}
	}

	mgr := tool.NewManager(dirs)
	states, err := mgr.ListInstalled()
	if err != nil {
		return fmt.Errorf("listing installed tools: %w", err)
	}

	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "gh tool reset will:")
	if len(states) == 0 {
		fmt.Fprintln(out, "  - remove 0 installed tools")
	} else {
		fmt.Fprintf(out, "  - remove %d installed tool(s): ", len(states))
		names := make([]string, len(states))
		for i, s := range states {
			names[i] = s.AsTool().Name()
		}
		fmt.Fprintln(out, strings.Join(names, ", "))
	}
	fmt.Fprintln(out, "  - delete:")
	for _, p := range wipeTargets {
		fmt.Fprintf(out, "      %s\n", p)
	}
	fmt.Fprintln(out, "  - preserve:")
	fmt.Fprintf(out, "      %s\n", dirs.ConfigFile())
	fmt.Fprintln(out)
	fmt.Fprintln(out, "The gh-tool extension itself is not removed. To finish, run:")
	fmt.Fprintln(out, "    gh extension remove ascarter/gh-tool")
	fmt.Fprintln(out)

	if !flagResetYes {
		if !ui.IsTTY() {
			return errors.New("refusing to run without confirmation on a non-interactive terminal; re-run with --yes")
		}
		if !confirm(cmd.InOrStdin(), out, "Proceed? [y/N] ") {
			fmt.Fprintln(out, "Aborted.")
			return nil
		}
	}

	// Remove each installed tool through Manager so symlinks/state/cache
	// clear through the same code path the user already trusts.
	if len(states) > 0 {
		mgr.SetReporter(ui.NewLineReporter(len(states) > 1, false))
		for _, s := range states {
			_ = mgr.Remove(s.AsTool())
		}
	}

	// Wipe directories wholesale — picks up any stragglers that Remove
	// missed (orphaned tool dirs, partial caches, etc.).
	for _, p := range wipeTargets {
		if err := os.RemoveAll(p); err != nil {
			fmt.Fprintf(out, "%s failed to remove %s: %s\n", ui.Error(ui.IconFailure), p, err)
		}
	}

	fmt.Fprintf(out, "\n%s gh-tool reset. Manifest preserved at %s.\n", ui.Success(ui.IconSuccess), dirs.ConfigFile())
	fmt.Fprintln(out, "To also remove the extension: gh extension remove ascarter/gh-tool")
	return nil
}

// pathSafe rejects a path that is empty, too short, or that doesn't contain
// a "gh-tool" segment — a belt-and-braces guard against a misresolved Dirs
// sending os.RemoveAll at something it shouldn't touch.
func pathSafe(p string) error {
	if strings.TrimSpace(p) == "" {
		return errors.New("empty path")
	}
	clean := filepath.Clean(p)
	if clean == "/" || clean == "." {
		return errors.New("refusing root-like path")
	}
	segments := strings.Split(clean, string(filepath.Separator))
	found := false
	for _, s := range segments {
		if s == appSegment {
			found = true
			break
		}
	}
	if !found {
		return errors.New("path does not contain a gh-tool segment")
	}
	return nil
}

// confirm reads a single y/N line from r, prompting to out.
func confirm(r io.Reader, out io.Writer, prompt string) bool {
	_, _ = out.Write([]byte(prompt))
	br := bufio.NewReader(r)
	line, err := br.ReadString('\n')
	if err != nil {
		return false
	}
	resp := strings.ToLower(strings.TrimSpace(line))
	return resp == "y" || resp == "yes"
}
