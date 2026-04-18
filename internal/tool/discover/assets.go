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
	for _, a := range raw.Assets {
		if isSkippable(a.Name) {
			rel.Skipped = append(rel.Skipped, a)
			continue
		}
		key, variant, ok := Classify(a.Name)
		if !ok {
			rel.Skipped = append(rel.Skipped, a)
			continue
		}
		a.Platform = key
		a.Variant = variant
		rel.All = append(rel.All, a)
		rel.ByPlatform[key] = append(rel.ByPlatform[key], a)
	}
	return rel, nil
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
	{"i686", "386"},
	{"i386", "386"},
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

// Classify maps an asset filename to a (PlatformKey, variant). Returns ok=false
// when the asset can't be assigned to a platform (e.g., source bundles, docs).
func Classify(name string) (key PlatformKey, variant string, ok bool) {
	lower := strings.ToLower(name)
	tokenSet := tokenize(lower)

	var goos, goarch string
	// Iterate in declaration order so longer/more specific tokens win
	// (e.g. x86_64 before x86, macos before any "mac" prefix).
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

	if goos == "" || goarch == "" {
		return "", "", false
	}

	for _, v := range variantTokens {
		if tokenSet[v] {
			variant = v
			break
		}
	}

	return PlatformKey(goos + "_" + goarch), variant, true
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
