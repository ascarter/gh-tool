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

		// OS-only or arch-only assets (fnm style) — defaults applied:
		// OS-only assumes amd64; arch-only assumes linux.
		{"fnm-linux.zip", "linux_amd64", "", true},
		{"fnm-macos.zip", "darwin_amd64", "", true},
		{"fnm-windows.zip", "windows_amd64", "", true},
		{"fnm-arm64.zip", "linux_arm64", "", true},
		{"fnm-arm32.zip", "linux_arm", "", true},

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
