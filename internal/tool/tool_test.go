package tool

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/ascarter/gh-tool/internal/config"
	"github.com/ascarter/gh-tool/internal/paths"
)

func TestExpandPattern(t *testing.T) {
	tests := []struct {
		pattern  string
		wantOS   string
		wantArch string
	}{
		{"tool-{{os}}-{{arch}}.tar.gz", runtime.GOOS, normalizeArch(runtime.GOARCH)},
		{"tool-linux-amd64.tar.gz", "", ""},
		{"{{os}}-only", runtime.GOOS, ""},
	}

	for _, tt := range tests {
		got := ExpandPattern(tt.pattern, "")
		if tt.wantOS != "" {
			if got == tt.pattern {
				t.Errorf("ExpandPattern(%q) = %q, expected substitution", tt.pattern, got)
			}
		}
	}

	// Specific expansion test
	pattern := "tool-{{os}}-{{arch}}.tar.gz"
	got := ExpandPattern(pattern, "")
	expected := "tool-" + normalizeOS(runtime.GOOS) + "-" + normalizeArch(runtime.GOARCH) + ".tar.gz"
	if got != expected {
		t.Errorf("ExpandPattern(%q) = %q, want %q", pattern, got, expected)
	}
}

func TestNormalizeOS(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"darwin", "darwin"},
		{"linux", "linux"},
		{"windows", "windows"},
		{"freebsd", "freebsd"},
	}
	for _, tt := range tests {
		if got := normalizeOS(tt.input); got != tt.want {
			t.Errorf("normalizeOS(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNormalizeArch(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"amd64", "amd64"},
		{"arm64", "arm64"},
		{"386", "i386"},
		{"riscv64", "riscv64"},
	}
	for _, tt := range tests {
		if got := normalizeArch(tt.input); got != tt.want {
			t.Errorf("normalizeArch(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestPlatformName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"darwin", "macos"},
		{"linux", "linux"},
		{"windows", "windows"},
		{"freebsd", "freebsd"},
	}
	for _, tt := range tests {
		if got := platformName(tt.input); got != tt.want {
			t.Errorf("platformName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestGnuArch(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"arm64", "aarch64"},
		{"amd64", "x86_64"},
		{"386", "i686"},
		{"riscv64", "riscv64"},
	}
	for _, tt := range tests {
		if got := gnuArch(tt.input); got != tt.want {
			t.Errorf("gnuArch(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestPlatformTriple(t *testing.T) {
	tests := []struct {
		goos, goarch string
		want         string
	}{
		{"darwin", "arm64", "aarch64-apple-darwin"},
		{"darwin", "amd64", "x86_64-apple-darwin"},
		{"linux", "amd64", "x86_64-unknown-linux-gnu"},
		{"linux", "arm64", "aarch64-unknown-linux-gnu"},
		{"windows", "amd64", "x86_64-pc-windows-msvc"},
		{"linux", "386", "i686-unknown-linux-gnu"},
	}
	for _, tt := range tests {
		got := platformTriple(tt.goos, tt.goarch)
		if got != tt.want {
			t.Errorf("platformTriple(%q, %q) = %q, want %q", tt.goos, tt.goarch, got, tt.want)
		}
	}
}

func TestExpandPatternTriple(t *testing.T) {
	pattern := "tool-{{triple}}.tar.gz"
	got := ExpandPattern(pattern, "")
	want := "tool-" + platformTriple(runtime.GOOS, runtime.GOARCH) + ".tar.gz"
	if got != want {
		t.Errorf("ExpandPattern(%q) = %q, want %q", pattern, got, want)
	}
}

func TestExpandPatternPlatform(t *testing.T) {
	pattern := "tool-{{platform}}-{{arch}}.tar.gz"
	got := ExpandPattern(pattern, "")
	want := "tool-" + platformName(runtime.GOOS) + "-" + normalizeArch(runtime.GOARCH) + ".tar.gz"
	if got != want {
		t.Errorf("ExpandPattern(%q) = %q, want %q", pattern, got, want)
	}
}

func TestExpandPatternGnuArch(t *testing.T) {
	pattern := "tool-*.{{os}}.{{gnuarch}}.tar.gz"
	got := ExpandPattern(pattern, "")
	want := "tool-*." + normalizeOS(runtime.GOOS) + "." + gnuArch(runtime.GOARCH) + ".tar.gz"
	if got != want {
		t.Errorf("ExpandPattern(%q) = %q, want %q", pattern, got, want)
	}
}

func TestExpandPatternTag(t *testing.T) {
	pattern := "tool-{{tag}}-{{os}}-{{arch}}.tar.gz"
	got := ExpandPattern(pattern, "v1.2.3")
	want := "tool-v1.2.3-" + normalizeOS(runtime.GOOS) + "-" + normalizeArch(runtime.GOARCH) + ".tar.gz"
	if got != want {
		t.Errorf("ExpandPattern(%q, %q) = %q, want %q", pattern, "v1.2.3", got, want)
	}
}

func TestRemoveToolSymlinks(t *testing.T) {
	root := t.TempDir()
	dirs := paths.Dirs{
		Config: filepath.Join(root, "config"),
		Data:   filepath.Join(root, "data"),
		State:  filepath.Join(root, "state"),
		Cache:  filepath.Join(root, "cache"),
	}
	if err := dirs.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}
	mgr := NewManager(dirs)

	name := "mytool"
	toolDir := dirs.ToolDir(name)
	otherDir := filepath.Join(root, "other")
	for _, d := range []string{toolDir, otherDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}

	// Files inside the tool dir (real targets).
	toolBin := filepath.Join(toolDir, "mytool-old")
	newBin := filepath.Join(toolDir, "mytool-new")
	manSrc := filepath.Join(toolDir, "mytool.1")
	compSrc := filepath.Join(toolDir, "_mytool")
	bashCompSrc := filepath.Join(toolDir, "mytool.bash")
	for _, f := range []string{toolBin, newBin, manSrc, compSrc, bashCompSrc} {
		if err := os.WriteFile(f, []byte("x"), 0o755); err != nil {
			t.Fatalf("write %s: %v", f, err)
		}
	}
	// Unrelated target outside the tool dir.
	otherBin := filepath.Join(otherDir, "unrelated")
	if err := os.WriteFile(otherBin, []byte("x"), 0o755); err != nil {
		t.Fatalf("write other: %v", err)
	}

	// Symlinks pointing into the tool dir (should be removed).
	managedOldLink := filepath.Join(dirs.BinDir(), "old-name")
	managedNewLink := filepath.Join(dirs.BinDir(), "mytool")
	managedManLink := filepath.Join(dirs.ManDir(), "mytool.1")
	managedZshLink := filepath.Join(dirs.ZshCompletionDir(), "_mytool")
	managedBashLink := filepath.Join(dirs.BashCompletionDir(), "mytool.bash")
	for _, pair := range [][2]string{
		{toolBin, managedOldLink},
		{newBin, managedNewLink},
		{manSrc, managedManLink},
		{compSrc, managedZshLink},
		{bashCompSrc, managedBashLink},
	} {
		if err := os.Symlink(pair[0], pair[1]); err != nil {
			t.Fatalf("symlink %s -> %s: %v", pair[1], pair[0], err)
		}
	}

	// Symlink to unrelated target (should be preserved).
	preservedLink := filepath.Join(dirs.BinDir(), "unrelated")
	if err := os.Symlink(otherBin, preservedLink); err != nil {
		t.Fatalf("symlink unrelated: %v", err)
	}

	// Plain file in BinDir (user's own binary; should be preserved).
	userFile := filepath.Join(dirs.BinDir(), "user-owned")
	if err := os.WriteFile(userFile, []byte("x"), 0o755); err != nil {
		t.Fatalf("write user file: %v", err)
	}

	mgr.removeToolSymlinks(name)

	// Managed links should be gone.
	for _, p := range []string{managedOldLink, managedNewLink, managedManLink, managedZshLink, managedBashLink} {
		if _, err := os.Lstat(p); !os.IsNotExist(err) {
			t.Errorf("expected %s removed, got err=%v", p, err)
		}
	}
	// Unrelated link and user file must remain.
	if _, err := os.Lstat(preservedLink); err != nil {
		t.Errorf("expected %s preserved, got err=%v", preservedLink, err)
	}
	if _, err := os.Lstat(userFile); err != nil {
		t.Errorf("expected %s preserved, got err=%v", userFile, err)
	}
}

func TestRemoveToolSymlinksRelativeTarget(t *testing.T) {
	root := t.TempDir()
	dirs := paths.Dirs{
		Config: filepath.Join(root, "config"),
		Data:   filepath.Join(root, "data"),
		State:  filepath.Join(root, "state"),
		Cache:  filepath.Join(root, "cache"),
	}
	if err := dirs.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}
	mgr := NewManager(dirs)

	name := "reltool"
	toolDir := dirs.ToolDir(name)
	if err := os.MkdirAll(toolDir, 0o755); err != nil {
		t.Fatalf("mkdir tool: %v", err)
	}
	target := filepath.Join(toolDir, "reltool")
	if err := os.WriteFile(target, []byte("x"), 0o755); err != nil {
		t.Fatalf("write target: %v", err)
	}

	link := filepath.Join(dirs.BinDir(), "reltool")
	rel, err := filepath.Rel(dirs.BinDir(), target)
	if err != nil {
		t.Fatalf("rel: %v", err)
	}
	if err := os.Symlink(rel, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	mgr.removeToolSymlinks(name)

	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Errorf("expected %s removed (relative symlink into tool dir), got err=%v", link, err)
	}
}

func TestInstalledStateRoundTrip(t *testing.T) {
	root := t.TempDir()
	dirs := paths.Dirs{
		Config: filepath.Join(root, "config"),
		Data:   filepath.Join(root, "data"),
		State:  filepath.Join(root, "state"),
		Cache:  filepath.Join(root, "cache"),
	}
	if err := dirs.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}
	mgr := NewManager(dirs)

	in := InstalledState{
		Repo:        "owner/widget",
		Tag:         "v1.2.3",
		Pattern:     "widget-darwin-arm64.tar.gz",
		Bin:         []string{"widget"},
		Man:         []string{"man/widget.1"},
		Completions: []string{"completions/_widget"},
		InstalledAt: "2026-04-17T12:00:00Z",
	}
	if err := mgr.writeState("widget", in); err != nil {
		t.Fatalf("writeState: %v", err)
	}
	got := mgr.ReadState("widget")
	if got == nil {
		t.Fatalf("ReadState returned nil")
	}
	if !reflect.DeepEqual(*got, in) {
		t.Errorf("round-trip mismatch:\n got=%#v\nwant=%#v", *got, in)
	}
}

func TestListInstalled(t *testing.T) {
	root := t.TempDir()
	dirs := paths.Dirs{
		Config: filepath.Join(root, "config"),
		Data:   filepath.Join(root, "data"),
		State:  filepath.Join(root, "state"),
		Cache:  filepath.Join(root, "cache"),
	}
	if err := dirs.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}
	mgr := NewManager(dirs)

	for _, s := range []InstalledState{
		{Repo: "z/zoo", Tag: "v1"},
		{Repo: "a/apple", Tag: "v2"},
		{Repo: "m/mango", Tag: "v3"},
	} {
		name := s.Repo
		// derive name from repo's "/" suffix the way Manager does internally
		for i := 0; i < len(s.Repo); i++ {
			if s.Repo[i] == '/' {
				name = s.Repo[i+1:]
				break
			}
		}
		if err := mgr.writeState(name, s); err != nil {
			t.Fatalf("writeState %s: %v", name, err)
		}
	}

	// A non-state file in the state dir should be ignored.
	if err := os.WriteFile(filepath.Join(dirs.State, "README"), []byte("hi"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}

	got, err := mgr.ListInstalled()
	if err != nil {
		t.Fatalf("ListInstalled: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 states, got %d", len(got))
	}
	wantOrder := []string{"a/apple", "m/mango", "z/zoo"}
	for i, w := range wantOrder {
		if got[i].Repo != w {
			t.Errorf("position %d: got %q, want %q", i, got[i].Repo, w)
		}
	}
}

func TestListInstalledEmpty(t *testing.T) {
	root := t.TempDir()
	dirs := paths.Dirs{State: filepath.Join(root, "state")}
	mgr := NewManager(dirs)
	got, err := mgr.ListInstalled()
	if err != nil {
		t.Fatalf("ListInstalled: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0, got %d", len(got))
	}
}

func TestBackfillState(t *testing.T) {
	state := &InstalledState{
		Repo: "owner/widget",
		Tag:  "v1",
	}
	manifest := &config.Tool{
		Repo:        "owner/widget",
		Pattern:     "widget-{{os}}-{{arch}}.tar.gz",
		Bin:         []string{"widget"},
		Man:         []string{"man/widget.1"},
		Completions: []string{"_widget"},
	}
	BackfillState(state, manifest)

	if !reflect.DeepEqual(state.Bin, manifest.Bin) {
		t.Errorf("Bin not backfilled")
	}
	if !reflect.DeepEqual(state.Man, manifest.Man) {
		t.Errorf("Man not backfilled")
	}
	if !reflect.DeepEqual(state.Completions, manifest.Completions) {
		t.Errorf("Completions not backfilled")
	}
}

func TestBackfillStatePreservesExisting(t *testing.T) {
	state := &InstalledState{
		Repo: "owner/widget",
		Bin:  []string{"kept-bin"},
	}
	manifest := &config.Tool{
		Repo: "owner/widget",
		Bin:  []string{"ignored"},
		Man:  []string{"filled-in"},
	}
	BackfillState(state, manifest)
	if !reflect.DeepEqual(state.Bin, []string{"kept-bin"}) {
		t.Errorf("Bin overwritten: %v", state.Bin)
	}
	if !reflect.DeepEqual(state.Man, []string{"filled-in"}) {
		t.Errorf("Man not backfilled when empty: %v", state.Man)
	}
}

func TestInstalledStateAsTool(t *testing.T) {
	s := InstalledState{
		Repo:    "owner/widget",
		Tag:     "v1",
		Pattern: "widget-darwin-arm64.tar.gz",
		Bin:     []string{"widget"},
	}
	got := s.AsTool()
	if got.Repo != s.Repo || got.Tag != s.Tag || got.Pattern != s.Pattern {
		t.Errorf("AsTool mismatch: %#v", got)
	}
	if !reflect.DeepEqual(got.Bin, s.Bin) {
		t.Errorf("AsTool.Bin mismatch")
	}
}

func TestParseBinSpec(t *testing.T) {
	tests := []struct {
		spec       string
		wantSource string
		wantLink   string
	}{
		{"fzf", "fzf", "fzf"},
		{"jq-macos-arm64:jq", "jq-macos-arm64", "jq"},
		{"yq_darwin_arm64:yq", "yq_darwin_arm64", "yq"},
		{"bin/tool:tool", "bin/tool", "tool"},
		{":bad", ":bad", ":bad"},
		{"bad:", "bad:", "bad:"},
	}
	for _, tt := range tests {
		src, link := parseBinSpec(tt.spec)
		if src != tt.wantSource || link != tt.wantLink {
			t.Errorf("parseBinSpec(%q) = (%q, %q), want (%q, %q)", tt.spec, src, link, tt.wantSource, tt.wantLink)
		}
	}
}

func TestFindFileInDir(t *testing.T) {
	root := t.TempDir()
	// Layout:
	//
	//	root/bin/foo
	//	root/share/man/man1/foo.1
	must := func(p string) {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	binPath := filepath.Join(root, "bin", "foo")
	manPath := filepath.Join(root, "share", "man", "man1", "foo.1")
	must(binPath)
	must(manPath)

	// Exact relative path.
	if got := findFileInDir(root, "bin/foo"); got != binPath {
		t.Errorf("exact: got %q want %q", got, binPath)
	}
	// Basename walk.
	if got := findFileInDir(root, "foo.1"); got != manPath {
		t.Errorf("basename: got %q want %q", got, manPath)
	}
	// Missing.
	if got := findFileInDir(root, "does-not-exist"); got != "" {
		t.Errorf("missing: got %q want empty", got)
	}
}

func TestFindDownloadedAsset(t *testing.T) {
	root := t.TempDir()
	// Mix of skippable and a real asset.
	files := map[string]bool{
		"tool-checksums.txt": true, // skippable
		"tool.sha256":        true,
		"tool.tar.gz.sig":    true,
		"tool.tar.gz.asc":    true,
		"tool.tar.gz":        false, // real
	}
	for name := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := findDownloadedAsset(root)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if filepath.Base(got) != "tool.tar.gz" {
		t.Errorf("got %q, want tool.tar.gz", got)
	}

	// Empty directory -> error.
	empty := t.TempDir()
	if _, err := findDownloadedAsset(empty); err == nil {
		t.Errorf("expected error on empty dir")
	}

	// Only skippable files -> error.
	skipOnly := t.TempDir()
	for _, n := range []string{"foo.sha256", "bar.sig"} {
		if err := os.WriteFile(filepath.Join(skipOnly, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := findDownloadedAsset(skipOnly); err == nil {
		t.Errorf("expected error when only skippable files present")
	}
}
