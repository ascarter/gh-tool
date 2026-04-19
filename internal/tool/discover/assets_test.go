package discover

import "testing"

func TestClassify(t *testing.T) {
	cases := []struct {
		name      string
		wantKey   PlatformKey
		wantVar   string
		wantOK    bool
	}{
		// Rust triples
		{"bat-v0.24.0-x86_64-apple-darwin.tar.gz", "darwin_amd64", "", true},
		{"bat-v0.24.0-aarch64-apple-darwin.tar.gz", "darwin_arm64", "", true},
		{"bat-v0.24.0-x86_64-unknown-linux-gnu.tar.gz", "linux_amd64", "gnu", true},
		{"bat-v0.24.0-x86_64-unknown-linux-musl.tar.gz", "linux_amd64", "musl", true},
		{"bat-v0.24.0-x86_64-pc-windows-msvc.zip", "windows_amd64", "msvc", true},

		// Underscore-glued (fzf, gh)
		{"fzf-0.46.1-darwin_amd64.zip", "darwin_amd64", "", true},
		{"fzf-0.46.1-linux_amd64.tar.gz", "linux_amd64", "", true},
		{"gh_2.42.1_macOS_amd64.zip", "darwin_amd64", "", true},
		{"gh_2.42.1_linux_amd64.tar.gz", "linux_amd64", "", true},

		// Capitalized (lazygit goreleaser)
		{"lazygit_0.40.2_Darwin_arm64.tar.gz", "darwin_arm64", "", true},
		{"lazygit_0.40.2_Linux_x86_64.tar.gz", "linux_amd64", "", true},

		// Bare-ish names (jq style)
		{"jq-macos-amd64", "darwin_amd64", "", true},
		{"jq-linux-amd64", "linux_amd64", "", true},
		{"jq-windows-amd64.exe", "windows_amd64", "", true},

		// OS-only or arch-only assets in isolation must NOT classify by
		// themselves anymore — defaulting now lives in classifyAssets,
		// which only fires when no sibling asset explicitly covers the
		// same OS or arch. See TestClassifyAssetsTwoPass for batched
		// behavior.
		{"fnm-linux.zip", "", "", false},
		{"fnm-macos.zip", "", "", false},
		{"fnm-windows.zip", "", "", false},
		{"fnm-arm64.zip", "", "", false},
		{"fnm-arm32.zip", "", "", false},

		// Unsupported architectures must NOT fall back to the OS-only
		// amd64 default (uv ships powerpc/riscv/s390x variants).
		{"uv-powerpc64le-unknown-linux-gnu.tar.gz", "", "", false},
		{"uv-riscv64gc-unknown-linux-gnu.tar.gz", "", "", false},
		{"uv-riscv64gc-unknown-linux-musl.tar.gz", "", "", false},
		{"uv-s390x-unknown-linux-gnu.tar.gz", "", "", false},
		// fzf ships loong64 and android variants; they must not be
		// classified as linux_amd64 / linux_arm64 respectively.
		{"fzf-0.71.0-linux_loong64.tar.gz", "", "", false},
		{"fzf-0.71.0-android_arm64.tar.gz", "", "", false},
		// armv5 IS a valid arm target — bucket it under linux_arm so
		// users targeting 32-bit arm can pick it; the linux_amd64 user
		// just ignores the bucket.
		{"fzf-0.71.0-linux_armv5.tar.gz", "linux_arm", "", true},
		// Bare "386" should map to GOARCH=386 (yq style:
		// yq_linux_386.tar.gz) rather than defaulting to amd64.
		{"yq_linux_386.tar.gz", "linux_386", "", true},

		// Should be skipped (not classified as platform)
		{"bat-v0.24.0.tar.gz", "", "", false},  // source-ish
		{"README.md", "", "", false},
	}
	for _, tc := range cases {
		gotKey, gotVar, ok := Classify(tc.name)
		if ok != tc.wantOK {
			t.Errorf("Classify(%q) ok = %v, want %v", tc.name, ok, tc.wantOK)
			continue
		}
		if gotKey != tc.wantKey {
			t.Errorf("Classify(%q) key = %q, want %q", tc.name, gotKey, tc.wantKey)
		}
		if gotVar != tc.wantVar {
			t.Errorf("Classify(%q) variant = %q, want %q", tc.name, gotVar, tc.wantVar)
		}
	}
}

func TestIsSkippable(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"bat-v0.24.0-x86_64-apple-darwin.tar.gz", false},
		{"checksums.txt", true},
		{"bat-v0.24.0-x86_64-apple-darwin.tar.gz.sha256", true},
		{"bat-v0.24.0-x86_64-apple-darwin.tar.gz.sig", true},
		{"bat_0.24.0_amd64.deb", true},
		{"bat-0.24.0-1.x86_64.rpm", true},
		{"installer.dmg", true},
		{"installer.msi", true},
		{"tool.AppImage", true},
		{"bat.sbom.json", true},
	}
	for _, tc := range cases {
		got := isSkippable(tc.name)
		if got != tc.want {
			t.Errorf("isSkippable(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestPreferArchives(t *testing.T) {
// yq ships both a bare binary and a tar.gz per platform; we should
// keep only the archive in ByPlatform.
in := []Asset{
{Name: "yq_linux_amd64"},
{Name: "yq_linux_amd64.tar.gz"},
}
out := preferArchives(in)
if len(out) != 1 || out[0].Name != "yq_linux_amd64.tar.gz" {
t.Errorf("preferArchives = %+v, want only the tar.gz", out)
}

// Two archives with the same stem (different ext): keep both — user
// can pick. We don't try to rank archive formats.
in = []Asset{
{Name: "yq_linux_amd64.tar.gz"},
{Name: "yq_linux_amd64.zip"},
}
out = preferArchives(in)
if len(out) != 2 {
t.Errorf("preferArchives stripped distinct archives: %+v", out)
}

// No archive present → keep the bare binary.
in = []Asset{{Name: "fnm-linux"}}
out = preferArchives(in)
if len(out) != 1 {
t.Errorf("preferArchives dropped lone bare binary: %+v", out)
}
}

func TestClassifyAssetsTwoPass(t *testing.T) {
t.Run("fnm: darwin OS-only asset expands to both archs", func(t *testing.T) {
in := []Asset{
{Name: "fnm-linux.zip"},
{Name: "fnm-macos.zip"},
{Name: "fnm-windows.zip"},
{Name: "fnm-arm64.zip"},
{Name: "fnm-arm32.zip"},
}
got, _ := classifyAssets(in)
// fnm-macos.zip is tentatively expanded to both darwin archs;
// cmd/add refines this against the actual Mach-O.
type kv struct{ name, plat string }
want := []kv{
{"fnm-linux.zip", "linux_amd64"},
{"fnm-macos.zip", "darwin_amd64"},
{"fnm-macos.zip", "darwin_arm64"},
{"fnm-windows.zip", "windows_amd64"},
{"fnm-arm64.zip", "linux_arm64"},
{"fnm-arm32.zip", "linux_arm"},
}
if len(got) != len(want) {
t.Fatalf("classified %d, want %d: %+v", len(got), len(want), got)
}
seen := map[kv]bool{}
for _, a := range got {
seen[kv{a.Name, string(a.Platform)}] = true
}
for _, w := range want {
if !seen[w] {
t.Errorf("missing classification %s -> %s", w.name, w.plat)
}
}
})

t.Run("darwin tentative expansion suppressed by explicit darwin-arm64 sibling", func(t *testing.T) {
// When a release ships an explicit darwin-arm64 asset alongside
// an OS-only macos asset, the OS-only asset is partial-skipped
// (osesWithExplicitArch guard) — no tentative expansion occurs.
in := []Asset{
{Name: "tool-macos.zip"},
{Name: "tool-darwin-arm64.zip"},
}
got, skipped := classifyAssets(in)
var sawMacosClassified bool
for _, a := range got {
if a.Name == "tool-macos.zip" {
sawMacosClassified = true
}
}
if sawMacosClassified {
t.Errorf("tool-macos.zip should be skipped when an explicit darwin sibling exists; got=%+v", got)
}
var sawSkipped bool
for _, a := range skipped {
if a.Name == "tool-macos.zip" {
sawSkipped = true
}
}
if !sawSkipped {
t.Errorf("expected tool-macos.zip in skipped, got %+v", skipped)
}
})

t.Run("fnm: defaults applied because no sibling has explicit arch", func(t *testing.T) {
in := []Asset{
{Name: "fnm-linux.zip"},
{Name: "fnm-macos.zip"},
{Name: "fnm-windows.zip"},
{Name: "fnm-arm64.zip"},
{Name: "fnm-arm32.zip"},
}
got, _ := classifyAssets(in)
// Linux/Windows OS-only assets keep the historical amd64 default.
linuxFound := false
windowsFound := false
for _, a := range got {
if a.Name == "fnm-linux.zip" && a.Platform == "linux_amd64" {
linuxFound = true
}
if a.Name == "fnm-windows.zip" && a.Platform == "windows_amd64" {
windowsFound = true
}
}
if !linuxFound {
t.Errorf("fnm-linux.zip should default to linux_amd64; got=%+v", got)
}
if !windowsFound {
t.Errorf("fnm-windows.zip should default to windows_amd64; got=%+v", got)
}
})

t.Run("yq: partial assets dropped because explicit siblings exist", func(t *testing.T) {
// yq_linux_386.tar.gz has no recognized arch token but a real
// yq_linux_amd64.tar.gz also exists — the partial must be
// skipped so it doesn't pollute linux_amd64.
in := []Asset{
{Name: "yq_linux_amd64.tar.gz"},
{Name: "yq_linux_arm64.tar.gz"},
{Name: "yq_linux_386.tar.gz"}, // wait — 386 IS in archTokens
{Name: "yq_linux_someunknownarch.tar.gz"},
}
got, skipped := classifyAssets(in)
var seenStray bool
for _, a := range got {
if a.Name == "yq_linux_someunknownarch.tar.gz" {
seenStray = true
}
}
if seenStray {
t.Errorf("partial asset with unknown arch was not skipped; got=%+v", got)
}
var skippedStray bool
for _, a := range skipped {
if a.Name == "yq_linux_someunknownarch.tar.gz" {
skippedStray = true
}
}
if !skippedStray {
t.Errorf("expected yq_linux_someunknownarch.tar.gz in skipped, got %+v", skipped)
}
})
}

func TestStripArchiveExt(t *testing.T) {
tests := []struct {
in, want string
}{
{"tool-linux-amd64.tar.gz", "tool-linux-amd64"},
{"tool-linux-amd64.tar.xz", "tool-linux-amd64"},
{"Tool.TAR.GZ", "Tool"},
{"tool-1.0.zip", "tool-1.0"},
{"tool.tgz", "tool"},
{"tool.txz", "tool"},
{"tool.tar.bz2", "tool"},
{"tool.tbz2", "tool"},
{"tool.tar.zst", "tool"},
{"tool", "tool"},
{"tool.gz", "tool.gz"},  // bare .gz not recognized
{"jq-macos-arm64", "jq-macos-arm64"},
}
for _, tt := range tests {
if got := StripArchiveExt(tt.in); got != tt.want {
t.Errorf("StripArchiveExt(%q)=%q want %q", tt.in, got, tt.want)
}
}
}
