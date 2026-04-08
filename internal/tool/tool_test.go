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
