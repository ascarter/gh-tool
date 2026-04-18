package discover

import (
	"reflect"
	"testing"
)

func TestFoldTriple(t *testing.T) {
	got := Fold("v0.24.0", map[PlatformKey]string{
		"darwin_amd64":  "bat-v0.24.0-x86_64-apple-darwin.tar.gz",
		"darwin_arm64":  "bat-v0.24.0-aarch64-apple-darwin.tar.gz",
		"linux_amd64":   "bat-v0.24.0-x86_64-unknown-linux-gnu.tar.gz",
		"linux_arm64":   "bat-v0.24.0-aarch64-unknown-linux-gnu.tar.gz",
		"windows_amd64": "bat-v0.24.0-x86_64-pc-windows-msvc.zip",
	})
	want := "bat-{{tag}}-{{triple}}.tar.gz"
	// Windows uses .zip extension so this won't fully fold — should fall back
	// to patterns.
	if got.Pattern != "" {
		t.Logf("unexpectedly folded with mixed extensions: %q", got.Pattern)
	}
	if len(got.Patterns) != 5 {
		t.Errorf("Fold returned %d patterns, want 5; got=%+v", len(got.Patterns), got)
	}
	_ = want
}

func TestFoldTripleNoWindows(t *testing.T) {
	got := Fold("v0.24.0", map[PlatformKey]string{
		"darwin_amd64": "bat-v0.24.0-x86_64-apple-darwin.tar.gz",
		"darwin_arm64": "bat-v0.24.0-aarch64-apple-darwin.tar.gz",
		"linux_amd64":  "bat-v0.24.0-x86_64-unknown-linux-gnu.tar.gz",
		"linux_arm64":  "bat-v0.24.0-aarch64-unknown-linux-gnu.tar.gz",
	})
	want := FoldResult{Pattern: "bat-{{tag}}-{{triple}}.tar.gz"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Fold = %+v, want %+v", got, want)
	}
}

func TestFoldOSGnuArch(t *testing.T) {
	// Underscore-glued, gnuarch style (lazygit-ish).
	got := Fold("0.40.2", map[PlatformKey]string{
		"darwin_amd64": "lazygit_0.40.2_Darwin_x86_64.tar.gz",
		"linux_amd64":  "lazygit_0.40.2_Linux_x86_64.tar.gz",
	})
	// Case mismatch (Darwin vs darwin literal): no fold available.
	if got.Pattern != "" {
		t.Errorf("Fold = %+v, expected fallback to patterns due to case", got)
	}
}

func TestFoldOSArch(t *testing.T) {
	// Simple OS + arch fold (jq-ish).
	got := Fold("1.7.1", map[PlatformKey]string{
		"darwin_amd64":  "jq-darwin-amd64",
		"darwin_arm64":  "jq-darwin-arm64",
		"linux_amd64":   "jq-linux-amd64",
		"linux_arm64":   "jq-linux-arm64",
	})
	want := FoldResult{Pattern: "jq-{{os}}-{{arch}}"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Fold = %+v, want %+v", got, want)
	}
}

func TestFoldExtensionMismatchFallsBack(t *testing.T) {
	// fzf: .zip on darwin, .tar.gz on linux — cannot fold to single pattern.
	got := Fold("0.46.1", map[PlatformKey]string{
		"darwin_amd64": "fzf-0.46.1-darwin_amd64.zip",
		"linux_amd64":  "fzf-0.46.1-linux_amd64.tar.gz",
	})
	if got.Pattern != "" {
		t.Errorf("expected patterns fallback, got pattern %q", got.Pattern)
	}
	if got.Patterns["darwin_amd64"] != "fzf-{{tag}}-darwin_amd64.zip" {
		t.Errorf("patterns map missing/wrong darwin_amd64: %+v", got.Patterns)
	}
	if got.Patterns["linux_amd64"] != "fzf-{{tag}}-linux_amd64.tar.gz" {
		t.Errorf("patterns map missing/wrong linux_amd64: %+v", got.Patterns)
	}
}

func TestFoldEmpty(t *testing.T) {
	got := Fold("v1", map[PlatformKey]string{})
	if got.Pattern != "" || got.Patterns != nil {
		t.Errorf("Fold(empty) = %+v, want zero", got)
	}
}

func TestFoldSinglePlatform(t *testing.T) {
	// One platform should always produce a folded pattern (trivially).
	got := Fold("v1.2.3", map[PlatformKey]string{
		"linux_amd64": "tool-v1.2.3-linux-amd64.tar.gz",
	})
	if got.Pattern == "" {
		t.Errorf("Fold(single) = %+v, expected a pattern", got)
	}
}
