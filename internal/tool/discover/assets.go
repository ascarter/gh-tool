// Package discover inspects a GitHub release to propose a manifest entry
// for gh-tool. It classifies release assets by platform, folds per-platform
// asset names into templated patterns, and inspects an extracted asset for
// binary/man/completion paths.
package discover

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	gh "github.com/cli/go-gh/v2"
)

// PlatformKey identifies a target platform as "goos_goarch", e.g. "linux_amd64".
type PlatformKey string

// String returns the platform key as a string.
func (p PlatformKey) String() string { return string(p) }

// GOOS returns the goos portion of the key.
func (p PlatformKey) GOOS() string {
	parts := strings.SplitN(string(p), "_", 2)
	return parts[0]
}

// GOARCH returns the goarch portion of the key.
func (p PlatformKey) GOARCH() string {
	parts := strings.SplitN(string(p), "_", 2)
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
}

// Asset is a single release asset.
type Asset struct {
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	URL      string `json:"url"`
	Platform PlatformKey
	Variant  string // optional disambiguator (e.g. "musl", "gnu", "static")
}

// Release is a fetched GitHub release with its assets classified.
type Release struct {
	Repo       string
	Tag        string
	All        []Asset
	ByPlatform map[PlatformKey][]Asset
	Skipped    []Asset
}

// FetchRelease loads a release via the gh CLI and classifies its assets.
// If tag is empty, the latest release is used.
func FetchRelease(repo, tag string) (*Release, error) {
	args := []string{"release", "view"}
	if tag != "" {
		args = append(args, tag)
	}
	args = append(args, "-R", repo, "--json", "tagName,assets")
	stdout, _, err := gh.Exec(args...)
	if err != nil {
		return nil, fmt.Errorf("fetching release %s: %w", repo, err)
	}

	var raw struct {
		TagName string  `json:"tagName"`
		Assets  []Asset `json:"assets"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &raw); err != nil {
		return nil, fmt.Errorf("parsing release JSON: %w", err)
	}

	rel := &Release{
		Repo:       repo,
		Tag:        raw.TagName,
		ByPlatform: map[PlatformKey][]Asset{},
	}
	classified, skipped := classifyAssets(raw.Assets)
	rel.Skipped = skipped
	for _, a := range classified {
		rel.All = append(rel.All, a)
		rel.ByPlatform[a.Platform] = append(rel.ByPlatform[a.Platform], a)
	}
	// Drop bare-binary assets that duplicate an archived sibling with the
	// same base name (e.g. yq ships both yq_linux_amd64 and
	// yq_linux_amd64.tar.gz). The archive is the canonical pick — it's
	// what 'gh tool add' inspects for binaries/man/completions and the
	// bare binary lives inside.
	for k, list := range rel.ByPlatform {
		rel.ByPlatform[k] = preferArchives(list)
	}
	return rel, nil
}

// classifyAssets buckets a release's assets via two-pass classification:
//
//  1. Pure detection runs per asset; fully-specified assets land in the
//     classified set immediately and contribute to the OS/arch coverage maps.
//  2. Partial assets (one of goos/goarch missing) are promoted via the
//     fnm-style default ONLY when no sibling asset already covers that
//     OS/arch explicitly. Otherwise they are skipped — this keeps yq-style
//     unrecognized arch assets out of the linux_amd64 bucket.
func classifyAssets(in []Asset) (classified, skipped []Asset) {
	type partial struct {
		asset   Asset
		goos    string
		goarch  string
		variant string
	}
	var partials []partial
	osesWithExplicitArch := map[string]bool{}
	archesWithExplicitOS := map[string]bool{}
	for _, a := range in {
		if isSkippable(a.Name) {
			skipped = append(skipped, a)
			continue
		}
		goos, goarch, variant, ok := detectTokens(a.Name)
		if !ok {
			skipped = append(skipped, a)
			continue
		}
		if goos != "" && goarch != "" {
			a.Platform = PlatformKey(goos + "_" + goarch)
			a.Variant = variant
			classified = append(classified, a)
			osesWithExplicitArch[goos] = true
			archesWithExplicitOS[goarch] = true
			continue
		}
		partials = append(partials, partial{asset: a, goos: goos, goarch: goarch, variant: variant})
	}
	for _, p := range partials {
		goos, goarch := p.goos, p.goarch
		if goos == "" {
			if archesWithExplicitOS[goarch] {
				skipped = append(skipped, p.asset)
				continue
			}
			goos = "linux"
		}
		if goarch == "" {
			if osesWithExplicitArch[goos] {
				skipped = append(skipped, p.asset)
				continue
			}
			goarch = "amd64"
		}
		p.asset.Platform = PlatformKey(goos + "_" + goarch)
		p.asset.Variant = p.variant
		classified = append(classified, p.asset)
	}
	return classified, skipped
}

// preferArchives drops bare-binary assets when an archived sibling with
// the same stem (asset name minus archive extension) is also present.
func preferArchives(list []Asset) []Asset {
	if len(list) < 2 {
		return list
	}
	stems := map[string]bool{}
	for _, a := range list {
		if stem, ok := archiveStem(a.Name); ok {
			stems[stem] = true
		}
	}
	if len(stems) == 0 {
		return list
	}
	out := make([]Asset, 0, len(list))
	for _, a := range list {
		if _, isArchive := archiveStem(a.Name); isArchive {
			out = append(out, a)
			continue
		}
		if stems[a.Name] {
			// This bare asset is shadowed by an archived sibling.
			continue
		}
		out = append(out, a)
	}
	return out
}

// archiveStem returns the asset name with a recognized archive extension
// removed and ok=true when the asset is an archive.
func archiveStem(name string) (string, bool) {
	low := strings.ToLower(name)
	for _, ext := range []string{".tar.gz", ".tar.xz", ".tar.bz2", ".tar.zst", ".tgz", ".txz", ".tbz2", ".zip"} {
		if strings.HasSuffix(low, ext) {
			return name[:len(name)-len(ext)], true
		}
	}
	return "", false
}

// Platforms returns the list of detected platforms, sorted.
func (r *Release) Platforms() []PlatformKey {
	out := make([]PlatformKey, 0, len(r.ByPlatform))
	for k := range r.ByPlatform {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// osTokens maps lowercase tokens that may appear in asset names to GOOS values.
// Order within a value list is the canonical/normalized form (used in PlatformKey).
var osTokens = []struct {
	token string
	goos  string
}{
	{"darwin", "darwin"},
	{"macos", "darwin"},
	{"apple", "darwin"},
	{"osx", "darwin"},
	{"linux", "linux"},
	{"windows", "windows"},
	{"win32", "windows"},
	{"win64", "windows"},
	{"freebsd", "freebsd"},
	{"openbsd", "openbsd"},
	{"netbsd", "netbsd"},
}

// archTokens maps lowercase tokens to GOARCH values.
var archTokens = []struct {
	token  string
	goarch string
}{
	{"x86_64", "amd64"},
	{"amd64", "amd64"},
	{"x64", "amd64"},
	{"aarch64", "arm64"},
	{"arm64", "arm64"},
	{"armv7", "arm"},
	{"armv6", "arm"},
	{"armv5", "arm"},
	{"armv4", "arm"},
	{"arm32", "arm"},
	{"arm", "arm"},
	{"i686", "386"},
	{"i386", "386"},
	{"386", "386"},
	{"x86", "386"},
}

// variantTokens picks up libc/build flavor markers (informational only).
var variantTokens = []string{"musl", "gnu", "static", "dynamic", "msvc"}

var skippableSuffixes = []string{
	".sha256", ".sha512", ".sha1", ".md5",
	".sig", ".asc", ".pem", ".cert", ".crt", ".pub",
	".sbom", ".spdx", ".intoto.jsonl",
	".deb", ".rpm", ".pkg", ".dmg", ".msi", ".apk", ".snap", ".AppImage",
}

var skippableContains = []string{
	"checksum", "checksums", "sha256sum",
	"-source.", "_source.",
	".sbom.",
}

// isSkippable returns true for assets that are not platform binaries:
// checksums, signatures, attestations, OS-installer packages, source
// tarballs, etc.
func isSkippable(name string) bool {
	lower := strings.ToLower(name)
	for _, s := range skippableSuffixes {
		if strings.HasSuffix(lower, strings.ToLower(s)) {
			return true
		}
	}
	for _, s := range skippableContains {
		if strings.Contains(lower, s) {
			return true
		}
	}
	// Source archives — generic name patterns
	if lower == "source.tar.gz" || lower == "source.zip" {
		return true
	}
	return false
}

// tokenBoundaryRE matches characters that always delimit tokens (dash, dot,
// slash, space). Underscores are preserved within a chunk so compound tokens
// like "x86_64" and "darwin_amd64" survive; we then also yield the
// underscore-split sub-tokens so simple "amd64" matches still work.
var tokenBoundaryRE = regexp.MustCompile(`[-./\\ ]`)

// unsupportedArchTokens are architecture markers we don't target. When any
// of these appear in an asset name, the asset is treated as for a different
// architecture (and not as an OS-only "default to amd64" candidate).
var unsupportedArchTokens = []string{
	"powerpc", "powerpc64", "powerpc64le", "ppc64", "ppc64le", "ppc",
	"riscv", "riscv64", "riscv64gc",
	"s390", "s390x",
	"mips", "mips64", "mipsel", "mips64el",
	"loongarch", "loongarch64", "loong64",
	"sparc", "sparc64",
}

// unsupportedOSTokens are operating systems we don't target by default for
// CLI tooling. Assets matching any of these are rejected so they don't get
// misclassified as Linux when no recognized OS token is present.
var unsupportedOSTokens = []string{
	"android", "ios", "illumos", "solaris", "plan9", "aix", "dragonfly", "haiku",
}

// hasUnsupportedArch reports whether any chunk of name contains a substring
// matching an unsupported arch marker. Substring (not whole-token) match
// because compound chunks like "powerpc64le" should still hit "powerpc".
func hasUnsupportedArch(tokenSet map[string]bool) bool {
	for chunk := range tokenSet {
		for _, tok := range unsupportedArchTokens {
			if strings.Contains(chunk, tok) {
				return true
			}
		}
	}
	return false
}

// hasUnsupportedOS reports whether any token in the set matches an OS we
// don't target. Whole-token match (not substring) so we don't reject e.g.
// "ios" inside arbitrary chunks.
func hasUnsupportedOS(tokenSet map[string]bool) bool {
	for _, tok := range unsupportedOSTokens {
		if tokenSet[tok] {
			return true
		}
	}
	return false
}

// Classify maps an asset filename to a (PlatformKey, variant). Returns ok=false
// when the asset can't be assigned to a platform on its own.
//
// This is a pure detection step: it does NOT default a missing goos or
// goarch. classifyAssets applies conditional defaults in a second pass so
// fnm-style OS-only releases work without misclassifying yq-style assets
// whose arch token we don't recognize.
func Classify(name string) (key PlatformKey, variant string, ok bool) {
	goos, goarch, variant, ok := detectTokens(name)
	if !ok || goos == "" || goarch == "" {
		return "", "", false
	}
	return PlatformKey(goos + "_" + goarch), variant, true
}

// detectTokens runs token detection without applying conventional defaults.
// Returns ok=false only when the asset has no recognizable tokens at all
// or matches an unsupported OS/arch.
func detectTokens(name string) (goos, goarch, variant string, ok bool) {
	lower := strings.ToLower(name)
	tokenSet := tokenize(lower)

	for _, at := range archTokens {
		if tokenSet[at.token] {
			goarch = at.goarch
			break
		}
	}
	for _, ot := range osTokens {
		if tokenSet[ot.token] {
			goos = ot.goos
			break
		}
	}

	if goos == "" && goarch == "" {
		return "", "", "", false
	}
	if goos == "" && hasUnsupportedOS(tokenSet) {
		return "", "", "", false
	}
	if goarch == "" && hasUnsupportedArch(tokenSet) {
		return "", "", "", false
	}

	for _, v := range variantTokens {
		if tokenSet[v] {
			variant = v
			break
		}
	}

	return goos, goarch, variant, true
}

// tokenize splits an asset filename into a set of lowercase tokens. Each
// dash/dot/slash-delimited chunk is added, plus all contiguous underscore-
// joined sub-spans of that chunk so compound arch tokens (like "x86_64"
// embedded in a longer chunk) are still recognized.
func tokenize(name string) map[string]bool {
	out := map[string]bool{}
	for _, chunk := range tokenBoundaryRE.Split(name, -1) {
		if chunk == "" {
			continue
		}
		out[chunk] = true
		if !strings.Contains(chunk, "_") {
			continue
		}
		parts := strings.Split(chunk, "_")
		// Yield every contiguous sub-span of underscore-separated parts.
		for i := 0; i < len(parts); i++ {
			for j := i + 1; j <= len(parts); j++ {
				sub := strings.Join(parts[i:j], "_")
				if sub != "" {
					out[sub] = true
				}
			}
		}
	}
	return out
}
