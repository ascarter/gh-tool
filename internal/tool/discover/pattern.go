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

	// Fallback: per-platform patterns map.
	patterns := make(map[string]string, len(selected))
	for k, v := range selected {
		patterns[string(k)] = v
	}
	return FoldResult{Patterns: patterns}
}

// candidateTemplates returns templates derived from one (asset, platform)
// example, in priority order: most-specific first.
func candidateTemplates(asset, tag string, key PlatformKey) []string {
	tokens := tool.Tokens(key.GOOS(), key.GOARCH(), tag)

	// Substitute the tag token first; it is independent of platform tokens.
	// Substitute longest tags first to avoid prefix collisions.
	tagBase := asset
	if tag != "" && strings.Contains(asset, tag) {
		tagBase = strings.ReplaceAll(asset, tag, "{{tag}}")
	}

	// Strategies — each substitutes one platform-token combination.
	type strategy struct {
		subs [][2]string // [token-name, literal-value] in substitution order
	}
	strategies := []strategy{
		// Most specific: the full Rust triple.
		{subs: [][2]string{{"{{triple}}", tokens["{{triple}}"]}}},
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
// result equals the literal asset name selected for that platform. This
// is the required exact round-trip check.
func verifyTemplate(template, tag string, selected map[PlatformKey]string) bool {
	for key, asset := range selected {
		expanded := tool.ExpandPatternFor(template, tag, key.GOOS(), key.GOARCH())
		if expanded != asset {
			return false
		}
	}
	return true
}
