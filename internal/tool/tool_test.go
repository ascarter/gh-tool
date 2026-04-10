package tool

import (
	"runtime"
	"testing"
)

func TestExpandPattern(t *testing.T) {
	tests := []struct {
		pattern string
		wantOS  string
		wantArch string
	}{
		{"tool-{{os}}-{{arch}}.tar.gz", runtime.GOOS, normalizeArch(runtime.GOARCH)},
		{"tool-linux-amd64.tar.gz", "", ""},
		{"{{os}}-only", runtime.GOOS, ""},
	}

	for _, tt := range tests {
		got := ExpandPattern(tt.pattern)
		if tt.wantOS != "" {
			if got == tt.pattern {
				t.Errorf("ExpandPattern(%q) = %q, expected substitution", tt.pattern, got)
			}
		}
	}

	// Specific expansion test
	pattern := "tool-{{os}}-{{arch}}.tar.gz"
	got := ExpandPattern(pattern)
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
	got := ExpandPattern(pattern)
	want := "tool-" + platformTriple(runtime.GOOS, runtime.GOARCH) + ".tar.gz"
	if got != want {
		t.Errorf("ExpandPattern(%q) = %q, want %q", pattern, got, want)
	}
}

func TestExpandPatternPlatform(t *testing.T) {
	pattern := "tool-{{platform}}-{{arch}}.tar.gz"
	got := ExpandPattern(pattern)
	want := "tool-" + platformName(runtime.GOOS) + "-" + normalizeArch(runtime.GOARCH) + ".tar.gz"
	if got != want {
		t.Errorf("ExpandPattern(%q) = %q, want %q", pattern, got, want)
	}
}

func TestExpandPatternGnuArch(t *testing.T) {
	pattern := "tool-*.{{os}}.{{gnuarch}}.tar.gz"
	got := ExpandPattern(pattern)
	want := "tool-*." + normalizeOS(runtime.GOOS) + "." + gnuArch(runtime.GOARCH) + ".tar.gz"
	if got != want {
		t.Errorf("ExpandPattern(%q) = %q, want %q", pattern, got, want)
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
		{":bad", ":bad", ":bad"},       // leading colon, no valid split
		{"bad:", "bad:", "bad:"},         // trailing colon, no valid split
	}
	for _, tt := range tests {
		src, link := parseBinSpec(tt.spec)
		if src != tt.wantSource || link != tt.wantLink {
			t.Errorf("parseBinSpec(%q) = (%q, %q), want (%q, %q)", tt.spec, src, link, tt.wantSource, tt.wantLink)
		}
	}
}
