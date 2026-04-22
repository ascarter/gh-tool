package cmd

import (
	"testing"

	"github.com/ascarter/gh-tool/internal/tool/discover"
)

func TestFoldBinName(t *testing.T) {
	tests := []struct {
		name string
		bin  string
		key  discover.PlatformKey
		want string
	}{
		{
			name: "os_arch pattern",
			bin:  "yq_linux_amd64",
			key:  "linux_amd64",
			want: "yq_{{os}}_{{arch}}",
		},
		{
			name: "os-arch with dashes",
			bin:  "tool-linux-arm64",
			key:  "linux_arm64",
			want: "tool-{{os}}-{{arch}}",
		},
		{
			name: "darwin platform name",
			bin:  "tool-macos-arm64",
			key:  "darwin_arm64",
			want: "tool-{{platform}}-{{arch}}",
		},
		{
			name: "gnuarch aarch64",
			bin:  "tool_linux_aarch64",
			key:  "linux_arm64",
			want: "tool_{{os}}_{{gnuarch}}",
		},
		{
			name: "no platform tokens",
			bin:  "yq",
			key:  "linux_amd64",
			want: "yq",
		},
		{
			name: "no fold when not delimited",
			bin:  "darwintools",
			key:  "darwin_amd64",
			want: "darwintools",
		},
		{
			name: "no fold when arch not delimited",
			bin:  "myamd64tool",
			key:  "linux_amd64",
			want: "myamd64tool",
		},
		{
			name: "windows exe stripped",
			bin:  "tool_windows_amd64",
			key:  "windows_amd64",
			want: "tool_{{os}}_{{arch}}",
		},
		{
			name: "dot delimited",
			bin:  "tool.linux.amd64",
			key:  "linux_amd64",
			want: "tool.{{os}}.{{arch}}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := foldBinName(tt.bin, tt.key)
			if got != tt.want {
				t.Errorf("foldBinName(%q, %q) = %q, want %q", tt.bin, tt.key, got, tt.want)
			}
		})
	}
}

func TestIsDelimitedToken(t *testing.T) {
	tests := []struct {
		s       string
		literal string
		want    bool
	}{
		{"yq_linux_amd64", "linux", true},
		{"darwintools", "darwin", false},
		{"myamd64tool", "amd64", false},
		{"linux-tool", "linux", true},
		{"tool.linux", "linux", true},
		{"linux", "linux", true},
	}

	for _, tt := range tests {
		t.Run(tt.s+"_"+tt.literal, func(t *testing.T) {
			got := isDelimitedToken(tt.s, tt.literal)
			if got != tt.want {
				t.Errorf("isDelimitedToken(%q, %q) = %v, want %v", tt.s, tt.literal, got, tt.want)
			}
		})
	}
}
