package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/BurntSushi/toml"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/ascarter/gh-tool/internal/config"
	"github.com/ascarter/gh-tool/internal/tool/discover"
)

var addCmd = &cobra.Command{
	Use:   "add <owner/repo>",
	Short: "Add a tool to the manifest interactively",
	Long: `Discover a release on GitHub, choose its assets and binary layout
interactively, then write the resulting [[tool]] block into the manifest.

Use this when you want to add a new tool without hand-editing the manifest.
The entry is not installed automatically — run "gh tool install <repo>"
afterwards.

For non-interactive use, run "gh tool install <repo> --pattern ..." with
the values you want.`,
	Args: cobra.ExactArgs(1),
	RunE: runAdd,
}

var (
	flagAddFile    string
	flagAddTag     string
	flagAddNoWrite bool
)

func init() {
	addCmd.Flags().StringVarP(&flagAddFile, "file", "f", "", "path to manifest file (default: $XDG_CONFIG_HOME/gh-tool/config.toml)")
	addCmd.Flags().StringVarP(&flagAddTag, "tag", "t", "", "release tag to inspect (default: latest)")
	addCmd.Flags().BoolVar(&flagAddNoWrite, "no-write", false, "print the generated [[tool]] block without writing the manifest")
	rootCmd.AddCommand(addCmd)
}

func runAdd(cmd *cobra.Command, args []string) error {
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return fmt.Errorf(`gh tool add requires an interactive terminal.
For non-interactive use, run:
  gh tool install %s --pattern '...' --bin '...'
or edit your manifest directly.`, args[0])
	}

	repo := args[0]
	if !strings.Contains(repo, "/") {
		return fmt.Errorf("expected owner/repo, got %q", repo)
	}

	dirs := resolveDirs()
	mfPath := manifestPath(dirs)

	cfg, err := config.Load(mfPath)
	if err != nil {
		return err
	}
	existing := cfg.FindTool(repo) != nil

	fmt.Printf("Fetching release for %s...\n", repo)
	rel, err := discover.FetchRelease(repo, flagAddTag)
	if err != nil {
		return err
	}
	fmt.Printf("✓ %s @ %s — %d classified assets, %d skipped\n", rel.Repo, rel.Tag, len(rel.All), len(rel.Skipped))

	platforms := rel.Platforms()
	if len(platforms) == 0 {
		return fmt.Errorf("no platform-classified assets in release %s", rel.Tag)
	}

	selectedPlatforms, err := chooseAddPlatforms(platforms)
	if err != nil {
		return err
	}

	chosen, err := chooseAddVariants(rel, selectedPlatforms)
	if err != nil {
		return err
	}

	fold := discover.Fold(rel.Tag, chosen)
	if err := confirmAddPattern(&fold, rel.Tag, chosen); err != nil {
		return err
	}

	hostKey := discover.PlatformKey(runtime.GOOS + "_" + runtime.GOARCH)
	hostAssetName, hostSupported := chosen[hostKey]
	var inspectAssetName string
	var inspectKey discover.PlatformKey
	if hostSupported {
		inspectAssetName = hostAssetName
		inspectKey = hostKey
	} else {
		// Pick first selected platform deterministically.
		keys := make([]discover.PlatformKey, 0, len(chosen))
		for k := range chosen {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
		inspectKey = keys[0]
		inspectAssetName = chosen[inspectKey]
		fmt.Printf("Host (%s/%s) has no asset for this tool; inspecting %s for layout.\n", runtime.GOOS, runtime.GOARCH, inspectKey)
	}

	fmt.Printf("Downloading %s for inspection...\n", inspectAssetName)
	tmpDir, err := os.MkdirTemp("", "gh-tool-add-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)
	assetPath, err := discover.DownloadAsset(repo, rel.Tag, inspectAssetName, tmpDir)
	if err != nil {
		return err
	}
	layout, err := discover.Inspect(assetPath)
	if err != nil {
		return err
	}

	bins, err := chooseAddBins(layout, repo, inspectAssetName, fold.Pattern)
	if err != nil {
		return err
	}
	mans, err := chooseAddPaths("man pages", layout.ManPages)
	if err != nil {
		return err
	}
	completions, err := chooseAddPaths("completions", layout.Completions)
	if err != nil {
		return err
	}

	t := config.Tool{
		Repo:        repo,
		Pattern:     fold.Pattern,
		Patterns:    fold.Patterns,
		Bin:         bins,
		Man:         mans,
		Completions: completions,
	}

	previewAddEntry(t)

	if flagAddNoWrite {
		fmt.Println("--no-write set; manifest unchanged.")
		return nil
	}

	prompt := "Save?"
	defaultYes := true
	if existing {
		prompt = "Overwrite existing entry?"
		defaultYes = false
	}
	var ok bool
	if err := survey.AskOne(&survey.Confirm{Message: prompt, Default: defaultYes}, &ok); err != nil {
		return err
	}
	if !ok {
		fmt.Println("Aborted.")
		return nil
	}

	cfg.AddOrUpdateTool(t)
	if err := config.Save(mfPath, cfg); err != nil {
		return fmt.Errorf("saving manifest: %w", err)
	}
	if existing {
		fmt.Printf("✓ Updated %s in %s\n", repo, mfPath)
	} else {
		fmt.Printf("✓ Saved %s to %s\n", repo, mfPath)
	}
	fmt.Printf("Run: gh tool install %s\n", repo)
	return nil
}

func chooseAddPlatforms(platforms []discover.PlatformKey) ([]discover.PlatformKey, error) {
	if len(platforms) == 1 {
		fmt.Printf("· Only one platform detected: %s\n", platforms[0])
		return platforms, nil
	}
	options := make([]string, len(platforms))
	defaults := []string{}
	host := runtime.GOOS + "_" + runtime.GOARCH
	commonPlatforms := map[string]bool{
		"darwin_amd64": true, "darwin_arm64": true,
		"linux_amd64": true, "linux_arm64": true,
		host: true,
	}
	for i, p := range platforms {
		options[i] = string(p)
		if commonPlatforms[string(p)] {
			defaults = append(defaults, string(p))
		}
	}
	var picked []string
	if err := survey.AskOne(&survey.MultiSelect{
		Message: "Select platforms to include:",
		Options: options,
		Default: defaults,
	}, &picked, survey.WithValidator(survey.MinItems(1))); err != nil {
		return nil, err
	}
	out := make([]discover.PlatformKey, 0, len(picked))
	for _, s := range picked {
		out = append(out, discover.PlatformKey(s))
	}
	return out, nil
}

func chooseAddVariants(rel *discover.Release, platforms []discover.PlatformKey) (map[discover.PlatformKey]string, error) {
	chosen := map[discover.PlatformKey]string{}
	for _, p := range platforms {
		assets := rel.ByPlatform[p]
		if len(assets) == 1 {
			chosen[p] = assets[0].Name
			continue
		}
		options := make([]string, len(assets))
		for i, a := range assets {
			options[i] = a.Name
		}
		var pick string
		if err := survey.AskOne(&survey.Select{
			Message: fmt.Sprintf("Choose asset for %s:", p),
			Options: options,
		}, &pick); err != nil {
			return nil, err
		}
		chosen[p] = pick
	}
	return chosen, nil
}

func confirmAddPattern(fold *discover.FoldResult, tag string, chosen map[discover.PlatformKey]string) error {
	if fold.Pattern != "" {
		fmt.Printf("Folded pattern: %s\n", fold.Pattern)
		fmt.Println("  Resolves to:")
		keys := make([]discover.PlatformKey, 0, len(chosen))
		for k := range chosen {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
		for _, k := range keys {
			fmt.Printf("    %s → %s\n", k, chosen[k])
		}
		// Skip confirmation when only one platform — it's trivially the asset name.
		if len(chosen) == 1 {
			return nil
		}
		var ok bool
		if err := survey.AskOne(&survey.Confirm{Message: "Use this pattern?", Default: true}, &ok); err != nil {
			return err
		}
		if !ok {
			// Fall back to per-platform map.
			fold.Pattern = ""
			fold.Patterns = make(map[string]string, len(chosen))
			for k, v := range chosen {
				fold.Patterns[string(k)] = v
			}
		}
		return nil
	}
	fmt.Println("Cannot fold into a single pattern; using per-platform patterns:")
	keys := make([]string, 0, len(fold.Patterns))
	for k := range fold.Patterns {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("  %s → %s\n", k, fold.Patterns[k])
	}
	return nil
}

// chooseAddBins prompts the user to pick which executables from the inspected
// archive should be symlinked. It also handles two refinements:
//
//   - bare-binary assets (e.g. jqlang/jq ships "jq-macos-arm64" as the asset
//     itself, not an archive). In that case the chosen bin's path equals the
//     inspected asset name, and we rewrite the source side to use the folded
//     pattern so it works across platforms.
//
//   - rename: if the resulting bin name does not match the repo name, offer
//     to rename it (writing "source:reponame" in the manifest).
func chooseAddBins(layout *discover.Layout, repo, inspectAssetName, foldedPattern string) ([]string, error) {
	_, name := splitRepoForAdd(repo)
	if len(layout.Executables) == 0 {
		fmt.Println("· No executables detected in archive; you may need to set --bin manually later.")
		return nil, nil
	}

	var picked []string
	if match := layout.MatchBinName(name); match != "" && len(layout.Executables) == 1 {
		fmt.Printf("· Auto-detected bin: %s\n", match)
		picked = []string{match}
	} else {
		options := make([]string, len(layout.Executables))
		defaults := []string{}
		for i, e := range layout.Executables {
			options[i] = e
			base := strings.TrimSuffix(strings.ToLower(filepath.Base(e)), ".exe")
			if base == strings.ToLower(name) {
				defaults = append(defaults, e)
			}
		}
		if err := survey.AskOne(&survey.MultiSelect{
			Message: "Select binaries to symlink:",
			Options: options,
			Default: defaults,
		}, &picked); err != nil {
			return nil, err
		}
	}

	out := make([]string, 0, len(picked))
	for _, p := range picked {
		source := strings.TrimSuffix(filepath.Base(p), ".exe")
		bareBinary := source == strings.TrimSuffix(inspectAssetName, ".exe")
		// Bare-binary case: the asset IS the executable. Use the folded
		// pattern (with extension stripped) as the source so per-platform
		// binaries resolve correctly. Skip when the fold produced a
		// per-platform map — there is no single template to embed.
		if bareBinary && foldedPattern != "" {
			source = stripArchiveExt(foldedPattern)
		}

		// Offer to rename only when there's a single binary and its
		// basename doesn't match the tool name. Multi-binary releases
		// (e.g. astral-sh/uv ships uv + uvx) should keep their original
		// names.
		linkName := source
		if len(picked) == 1 && !strings.EqualFold(filepath.Base(source), name) {
			var rename bool
			if err := survey.AskOne(&survey.Confirm{
				Message: fmt.Sprintf("Rename symlink %q to %q?", filepath.Base(source), name),
				Default: true,
			}, &rename); err != nil {
				return nil, err
			}
			if rename {
				linkName = name
			}
		}

		if linkName == source {
			out = append(out, source)
		} else {
			out = append(out, source+":"+linkName)
		}
	}
	return out, nil
}

// stripArchiveExt removes a single trailing archive extension from s. Used to
// derive a bare-binary name from a folded archive pattern.
func stripArchiveExt(s string) string {
	for _, ext := range []string{".tar.gz", ".tar.xz", ".tar.bz2", ".tgz", ".txz", ".zip", ".gz", ".xz", ".exe"} {
		if strings.HasSuffix(strings.ToLower(s), ext) {
			return s[:len(s)-len(ext)]
		}
	}
	return s
}

func chooseAddPaths(label string, found []string) ([]string, error) {
	if len(found) == 0 {
		return nil, nil
	}
	var include bool
	if err := survey.AskOne(&survey.Confirm{
		Message: fmt.Sprintf("Include %d %s?", len(found), label),
		Default: true,
	}, &include); err != nil {
		return nil, err
	}
	if !include {
		return nil, nil
	}
	if len(found) == 1 {
		return found, nil
	}
	var picked []string
	if err := survey.AskOne(&survey.MultiSelect{
		Message: fmt.Sprintf("Select %s to include:", label),
		Options: found,
		Default: found,
	}, &picked); err != nil {
		return nil, err
	}
	return picked, nil
}

func previewAddEntry(t config.Tool) {
	fmt.Println()
	fmt.Println("Generated entry:")
	fmt.Println("─────────────────────────────────────────")
	enc := toml.NewEncoder(os.Stdout)
	_ = enc.Encode(struct {
		Tool []config.Tool `toml:"tool"`
	}{Tool: []config.Tool{t}})
	fmt.Println("─────────────────────────────────────────")
}

func splitRepoForAdd(repo string) (owner, name string) {
	for i := range repo {
		if repo[i] == '/' {
			return repo[:i], repo[i+1:]
		}
	}
	return "", repo
}
