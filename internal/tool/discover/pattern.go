package discover

import (
	"sort"
	"strings"

	"github.com/ascarter/gh-tool/internal/tool"
)

// FoldResult is the outcome of trying to reduce per-platform asset names
// into a single templated pattern.
type FoldResult struct {
	// Pattern is the single template that round-trips for every platform
	// (empty if Patterns is set instead).
	Pattern string

	// Patterns maps "goos_goarch" to the literal asset name; populated when
	// no single template can express every selected asset.
	Patterns map[string]string
}

// Fold tries to reduce per-platform asset names to one templated pattern.
// The returned FoldResult has either Pattern set (single template) or
// Patterns set (per-platform map). It never has both.
//
// Tag is the resolved release tag; it is substituted as {{tag}} in the
// candidate template.
//
// The selected map is keyed by PlatformKey; each value is the literal
// asset name chosen for that platform.
func Fold(tag string, selected map[PlatformKey]string) FoldResult {
	if len(selected) == 0 {
		return FoldResult{}
	}

	// Iterate platforms in stable order so candidates are deterministic.
	keys := make([]PlatformKey, 0, len(selected))
	for k := range selected {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

	// Generate candidates from the first platform and verify each against
	// the rest. Return the first that round-trips for every platform.
	first := keys[0]
	for _, candidate := range candidateTemplates(selected[first], tag, first) {
		if verifyTemplate(candidate, tag, selected) {
			return FoldResult{Pattern: candidate}
		}
	}

	// Fallback: per-platform patterns map. Still substitute the tag in
	// each entry so upgrades pick up the new version automatically.
	patterns := make(map[string]string, len(selected))
	for k, v := range selected {
		patterns[string(k)] = substituteTag(v, tag)
	}
	return FoldResult{Patterns: patterns}
}

// substituteTag rewrites occurrences of the tag in s. The literal tag is
// replaced with {{tag}}; if the literal isn't present but the tag without a
// leading "v" is, that form is replaced with "*" (a glob wildcard accepted
// by gh release download). Both placements coexist for projects that mix
// the two styles.
func substituteTag(s, tag string) string {
	if tag == "" {
		return s
	}
	out := s
	if strings.Contains(out, tag) {
		out = strings.ReplaceAll(out, tag, "{{tag}}")
	}
	stripped := strings.TrimPrefix(tag, "v")
	if stripped != tag && stripped != "" && strings.Contains(out, stripped) {
		out = strings.ReplaceAll(out, stripped, "*")
	}
	return out
}

// candidateTemplates returns templates derived from one (asset, platform)
// example, in priority order: most-specific first.
func candidateTemplates(asset, tag string, key PlatformKey) []string {
	tokens := tool.Tokens(key.GOOS(), key.GOARCH(), tag)

	// Substitute the tag token first; it is independent of platform tokens.
	// Prefer the literal tag (round-trips exactly via {{tag}}); fall back
	// to the no-leading-"v" form substituted with "*" as a glob wildcard
	// (gh release download -p accepts globs). This handles projects whose
	// tag is "v1.2.3" but whose assets embed "1.2.3".
	tagBase := substituteTag(asset, tag)

	// Strategies — each substitutes one platform-token combination.
	type strategy struct {
		subs [][2]string // [token-name, literal-value] in substitution order
	}
	strategies := []strategy{
		// Most specific: the full Rust triple (Linux=gnu).
		{subs: [][2]string{{"{{triple}}", tokens["{{triple}}"]}}},
		// Rust triple with Linux=musl (uv, ruff, watchexec, …).
		{subs: [][2]string{{"{{musltriple}}", tokens["{{musltriple}}"]}}},
		// Platform name + GNU arch (handles e.g. "macos_x86_64", "linux_aarch64").
		{subs: [][2]string{
			{"{{platform}}", tokens["{{platform}}"]},
			{"{{gnuarch}}", tokens["{{gnuarch}}"]},
		}},
		// OS + GNU arch.
		{subs: [][2]string{
			{"{{os}}", tokens["{{os}}"]},
			{"{{gnuarch}}", tokens["{{gnuarch}}"]},
		}},
		// Platform name + release arch (x86_64 everywhere; aarch64 on macOS arm64, arm64 on Linux arm64).
		{subs: [][2]string{
			{"{{platform}}", tokens["{{platform}}"]},
			{"{{relarch}}", tokens["{{relarch}}"]},
		}},
		// OS + release arch.
		{subs: [][2]string{
			{"{{os}}", tokens["{{os}}"]},
			{"{{relarch}}", tokens["{{relarch}}"]},
		}},
		// Platform name + short arch (x64 for amd64, arm64 for arm64).
		{subs: [][2]string{
			{"{{platform}}", tokens["{{platform}}"]},
			{"{{shortarch}}", tokens["{{shortarch}}"]},
		}},
		// OS + short arch.
		{subs: [][2]string{
			{"{{os}}", tokens["{{os}}"]},
			{"{{shortarch}}", tokens["{{shortarch}}"]},
		}},
		// Platform name + Go arch.
		{subs: [][2]string{
			{"{{platform}}", tokens["{{platform}}"]},
			{"{{arch}}", tokens["{{arch}}"]},
		}},
		// OS + Go arch (most permissive).
		{subs: [][2]string{
			{"{{os}}", tokens["{{os}}"]},
			{"{{arch}}", tokens["{{arch}}"]},
		}},
	}

	out := make([]string, 0, len(strategies))
	for _, s := range strategies {
		candidate := tagBase
		applied := true
		for _, sub := range s.subs {
			tokenName, literal := sub[0], sub[1]
			if literal == "" || !strings.Contains(candidate, literal) {
				applied = false
				break
			}
			candidate = strings.ReplaceAll(candidate, literal, tokenName)
		}
		if applied {
			out = append(out, candidate)
		}
	}
	return out
}

// verifyTemplate expands the template for every platform and checks the
// result equals the literal asset name selected for that platform. A
// literal "*" in the template is treated as a glob (one or more chars,
// no slashes) so wildcard tag substitutions still round-trip.
func verifyTemplate(template, tag string, selected map[PlatformKey]string) bool {
	for key, asset := range selected {
		expanded := tool.ExpandPatternFor(template, tag, key.GOOS(), key.GOARCH())
		if !matchExpanded(expanded, asset) {
			return false
		}
	}
	return true
}

// matchExpanded compares an expanded pattern against an asset name,
// honoring "*" in the pattern as a non-empty, slash-free glob.
func matchExpanded(pattern, asset string) bool {
	if !strings.Contains(pattern, "*") {
		return pattern == asset
	}
	parts := strings.Split(pattern, "*")
	if !strings.HasPrefix(asset, parts[0]) {
		return false
	}
	rest := asset[len(parts[0]):]
	for i := 1; i < len(parts); i++ {
		seg := parts[i]
		if seg == "" {
			// Trailing "*" — accept any remaining non-empty content.
			if i == len(parts)-1 {
				return rest != "" && !strings.Contains(rest, "/")
			}
			continue
		}
		idx := strings.Index(rest, seg)
		if idx <= 0 {
			// Need at least one char before the next literal segment.
			return false
		}
		if strings.Contains(rest[:idx], "/") {
			return false
		}
		rest = rest[idx+len(seg):]
	}
	return rest == ""
}
