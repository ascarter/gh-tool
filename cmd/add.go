package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/ascarter/gh-tool/internal/config"
	"github.com/ascarter/gh-tool/internal/tool"
	"github.com/ascarter/gh-tool/internal/tool/discover"
	"github.com/ascarter/gh-tool/internal/ui"
)

var addCmd = &cobra.Command{
	Use:   "add <owner/repo>",
	Short: "Add a tool to the manifest interactively",
	Long: `Inspect a GitHub release, pick its assets and binary layout interactively,
and write a [[tool]] block to the manifest. Pass --install to also install
it in the same step.`,
	Args: cobra.ExactArgs(1),
	RunE: runAdd,
}

var (
	flagAddFile    string
	flagAddTag     string
	flagAddNoWrite bool
	flagAddInstall bool
)

func init() {
	addCmd.Flags().StringVarP(&flagAddFile, "file", "f", "", "manifest path (default: $XDG_CONFIG_HOME/gh-tool/config.toml)")
	addCmd.Flags().StringVarP(&flagAddTag, "tag", "t", "", "release tag (default: latest)")
	addCmd.Flags().BoolVar(&flagAddNoWrite, "no-write", false, "print the [[tool]] block without writing")
	addCmd.Flags().BoolVar(&flagAddInstall, "install", false, "install after writing the manifest entry")
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
		warnf("host (%s/%s) has no asset for this tool; inspecting %s for layout.", runtime.GOOS, runtime.GOARCH, inspectKey)
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

	// If the inspected asset is darwin and was tentatively expanded to
	// both archs (same asset name), use the Mach-O scan to drop the
	// arches the binary doesn't actually support. Skip when no Mach-O
	// was detected — the asset may be a script/JAR that runs everywhere.
	if inspectKey.GOOS() == "darwin" && len(layout.MachOArchs) > 0 {
		refineDarwinArchs(chosen, inspectAssetName, layout.MachOArchs)
	}

	fold := discover.Fold(rel.Tag, chosen)
	if err := confirmAddPattern(&fold, rel.Tag, chosen); err != nil {
		return err
	}

	bins, err := chooseAddBins(layout, repo, inspectAssetName, fold.Pattern, inspectKey)
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
	if err := promptConfirm(prompt, defaultYes, &ok); err != nil {
		return err
	}
	if !ok {
		warnf("%s not saved", repo)
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

	if !flagAddInstall {
		return nil
	}

	var doInstall bool
	if err := promptConfirm(fmt.Sprintf("Install %s now?", repo), true, &doInstall); err != nil {
		return err
	}
	if !doInstall {
		return nil
	}
	mgr := tool.NewManager(dirs).WithReporter(ui.NewLineReporter(false, true))
	mgr.CleanupInstall(t.Name())
	return mgr.Install(t, true)
}

// refineDarwinArchs uses a Mach-O architecture scan from the inspected
// asset to drop tentative darwin arches that the asset does not actually
// support. The classify step optimistically expands a darwin OS-only
// asset (e.g. fnm-macos.zip) into both darwin_amd64 and darwin_arm64;
// here we trim that down once we've actually seen the binary.
//
// Only entries in chosen that point at the same inspected asset name are
// affected — a manifest that selected a different asset for a sibling
// arch (e.g. an explicit darwin-arm64 binary alongside a universal one)
// is left untouched.
func refineDarwinArchs(chosen map[discover.PlatformKey]string, inspectAssetName string, machoArchs map[string]bool) {
	for _, arch := range []string{"amd64", "arm64"} {
		key := discover.PlatformKey("darwin_" + arch)
		if chosen[key] != inspectAssetName {
			continue
		}
		if !machoArchs[arch] {
			delete(chosen, key)
			warnf("dropping %s: inspected Mach-O does not contain %s slice", key, arch)
		}
	}
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
	if err := promptMultiSelect("Select platforms to include:", options, defaults, &picked); err != nil {
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

	// Pre-pass: when multiple platforms of the same OS each have a
	// libc/build variant choice (e.g. musl vs gnu on Linux), ask once
	// and apply to every platform in that OS group. Skips the noisy
	// per-platform prompt for the common case.
	preselected := preselectVariantPerOS(rel, platforms)

	for _, p := range platforms {
		assets := rel.ByPlatform[p]
		if len(assets) == 1 {
			chosen[p] = assets[0].Name
			continue
		}
		if name, ok := preselected[p]; ok {
			chosen[p] = name
			continue
		}
		options := make([]string, len(assets))
		for i, a := range assets {
			options[i] = a.Name
		}
		var pick string
		if err := promptSelect(fmt.Sprintf("Choose asset for %s:", p), options, &pick); err != nil {
			return nil, err
		}
		chosen[p] = pick
	}
	return chosen, nil
}

// preselectVariantPerOS prompts once per OS for the variant (musl/gnu/etc.)
// to use across every selected platform of that OS. Returns a map from
// PlatformKey to the chosen asset name. Platforms not covered (e.g., the OS
// has only one platform, or variants do not match across platforms) are
// omitted, and the caller falls back to per-platform prompting.
func preselectVariantPerOS(rel *discover.Release, platforms []discover.PlatformKey) map[discover.PlatformKey]string {
	out := map[discover.PlatformKey]string{}

	// Group platforms by OS, but only include ones with multiple assets.
	byOS := map[string][]discover.PlatformKey{}
	for _, p := range platforms {
		if len(rel.ByPlatform[p]) <= 1 {
			continue
		}
		byOS[p.GOOS()] = append(byOS[p.GOOS()], p)
	}

	for goos, group := range byOS {
		if len(group) < 2 {
			continue
		}

		// Intersect the variant sets across every platform in the group.
		// A variant is only offerable if every platform has an asset for it.
		variantsByPlatform := make([]map[string]string, 0, len(group))
		for _, p := range group {
			vmap := map[string]string{}
			for _, a := range rel.ByPlatform[p] {
				if a.Variant != "" {
					vmap[a.Variant] = a.Name
				}
			}
			variantsByPlatform = append(variantsByPlatform, vmap)
		}
		common := commonKeys(variantsByPlatform)
		if len(common) < 2 {
			// No meaningful choice to offer; fall back to per-platform prompts.
			continue
		}

		var pick string
		if err := promptSelect(fmt.Sprintf("Variant for %s (applies to %d platforms):", goos, len(group)), common, &pick); err != nil {
			// On error or interrupt, skip pre-selection; per-platform prompt
			// will run as a fallback.
			continue
		}
		for i, p := range group {
			out[p] = variantsByPlatform[i][pick]
		}
	}
	return out
}

// commonKeys returns keys present in every map, sorted.
func commonKeys(maps []map[string]string) []string {
	if len(maps) == 0 {
		return nil
	}
	out := []string{}
	for k := range maps[0] {
		ok := true
		for _, m := range maps[1:] {
			if _, found := m[k]; !found {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
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
		if err := promptConfirm("Use this pattern?", true, &ok); err != nil {
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
	warnf("Cannot fold into a single pattern; using per-platform patterns:")
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
func chooseAddBins(layout *discover.Layout, repo, inspectAssetName, foldedPattern string, inspectKey discover.PlatformKey) ([]string, error) {
	_, name := config.SplitRepo(repo)
	if len(layout.Executables) == 0 {
		warnf("no executables detected in archive; you may need to set bin manually later.")
		return nil, nil
	}

	var picked []string
	if match := layout.MatchBinName(name); match != "" && len(layout.Executables) == 1 {
		fmt.Printf("· Auto-detected bin: %s\n", match)
		picked = []string{match}
	} else {
		options := make([]string, len(layout.Executables))
		// Default to selecting every detected executable. Multi-binary
		// releases (uv: uv+uvx, git: many) almost always want all of
		// them, and a single-executable case where the name doesn't
		// match the repo (handled below) still wants the binary.
		for i, e := range layout.Executables {
			options[i] = e
		}
		if err := promptMultiSelect("Select binaries to symlink:", options, options, &picked); err != nil {
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
			source = stripBinaryExt(foldedPattern)
		} else if foldedPattern != "" {
			// Archive case: the executable inside the archive may have a
			// platform-specific name (e.g. "yq_linux_amd64"). Fold
			// platform tokens into template variables so the bin entry
			// works across platforms.
			source = foldBinName(source, inspectKey)
		}

		// Offer to rename only when there's a single binary, the basename
		// doesn't match the tool name, AND the basename contains a
		// separator (dash, underscore, or template braces). That last
		// condition skips the prompt for short ad-hoc names like
		// "btm" → "bottom" or "rg" → "ripgrep" while still catching
		// platform-encoded names like "jq-macos-arm64" or
		// "yq_{{os}}_{{arch}}".
		linkName := source
		base := filepath.Base(source)
		hasSep := strings.ContainsAny(base, "-_{")
		if len(picked) == 1 && hasSep && !strings.EqualFold(base, name) {
			var rename bool
			if err := promptConfirm(fmt.Sprintf("Rename symlink %q to %q?", base, name), true, &rename); err != nil {
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

// foldBinName replaces platform-specific literals in an executable name
// with template variables. For example, "yq_linux_amd64" inspected on
// linux_amd64 becomes "yq_{{os}}_{{arch}}". Only substitutes tokens that
// appear as delimited segments (bounded by start/end of string or
// separators like - _ .) to avoid false positives.
func foldBinName(name string, key discover.PlatformKey) string {
	tokens := tool.Tokens(key.GOOS(), key.GOARCH(), "")

	type sub struct {
		pairs [][2]string
	}
	strategies := []sub{
		{pairs: [][2]string{{"{{triple}}", tokens["{{triple}}"]}}},
		{pairs: [][2]string{{"{{musltriple}}", tokens["{{musltriple}}"]}}},
		// Prefer {{os}} over {{platform}} — the two are identical except
		// on darwin (os=darwin, platform=macos), and {{os}} aligns with
		// the Go naming convention used by most release assets.
		{pairs: [][2]string{
			{"{{os}}", tokens["{{os}}"]},
			{"{{gnuarch}}", tokens["{{gnuarch}}"]},
		}},
		{pairs: [][2]string{
			{"{{os}}", tokens["{{os}}"]},
			{"{{arch}}", tokens["{{arch}}"]},
		}},
		{pairs: [][2]string{
			{"{{platform}}", tokens["{{platform}}"]},
			{"{{gnuarch}}", tokens["{{gnuarch}}"]},
		}},
		{pairs: [][2]string{
			{"{{platform}}", tokens["{{platform}}"]},
			{"{{arch}}", tokens["{{arch}}"]},
		}},
	}

	for _, s := range strategies {
		candidate := name
		applied := true
		for _, pair := range s.pairs {
			tokenName, literal := pair[0], pair[1]
			if literal == "" || !isDelimitedToken(candidate, literal) {
				applied = false
				break
			}
			candidate = strings.Replace(candidate, literal, tokenName, 1)
		}
		if applied && candidate != name {
			return candidate
		}
	}
	return name
}

// isDelimitedToken returns true if literal appears in s as a
// delimiter-bounded segment (delimiters: start/end of string, '-', '_', '.').
func isDelimitedToken(s, literal string) bool {
	idx := strings.Index(s, literal)
	if idx < 0 {
		return false
	}
	before := idx == 0 || isBinDelim(s[idx-1])
	after := idx+len(literal) == len(s) || isBinDelim(s[idx+len(literal)])
	return before && after
}

func isBinDelim(b byte) bool {
	return b == '-' || b == '_' || b == '.'
}

// stripBinaryExt removes a single trailing extension that may appear on a
// bare-binary asset (archive extension or .exe). Used to derive a bare-
// binary symlink source from a folded archive pattern.
func stripBinaryExt(s string) string {
	stripped := discover.StripArchiveExt(s)
	if stripped != s {
		return stripped
	}
	if strings.HasSuffix(strings.ToLower(s), ".exe") {
		return s[:len(s)-len(".exe")]
	}
	return s
}

func chooseAddPaths(label string, found []string) ([]string, error) {
	if len(found) == 0 {
		return nil, nil
	}
	var picked []string
	title := fmt.Sprintf("Select %s to include (none to skip):", label)
	if err := promptMultiSelectOptional(title, found, found, &picked); err != nil {
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
