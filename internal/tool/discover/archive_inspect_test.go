package discover

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func writeFile(t *testing.T, path string, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func TestScanLayout(t *testing.T) {
	root := t.TempDir()

	// Top-level executable.
	writeFile(t, filepath.Join(root, "bat"), "#!/bin/sh\n", 0o755)
	// Nested executable in a single subdir.
	writeFile(t, filepath.Join(root, "subdir", "helper"), "#!/bin/sh\n", 0o755)
	// Deeply nested binary should be skipped.
	writeFile(t, filepath.Join(root, "deep", "very", "buried"), "#!/bin/sh\n", 0o755)
	// Non-executable regular file.
	writeFile(t, filepath.Join(root, "README.md"), "readme", 0o644)
	// Man page under man/man1/.
	writeFile(t, filepath.Join(root, "man", "man1", "bat.1"), "manpage", 0o644)
	// Bash, zsh, fish completions.
	writeFile(t, filepath.Join(root, "autocomplete", "bat.bash"), "complete", 0o644)
	writeFile(t, filepath.Join(root, "autocomplete", "_bat"), "zsh complete", 0o644)
	writeFile(t, filepath.Join(root, "autocomplete", "bat.fish"), "complete", 0o644)

	got, err := scanLayout(root)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(got.Executables)
	sort.Strings(got.ManPages)
	sort.Strings(got.Completions)

	wantExe := []string{"bat", "subdir/helper"}
	if !reflect.DeepEqual(got.Executables, wantExe) {
		t.Errorf("Executables = %q, want %q", got.Executables, wantExe)
	}
	wantMan := []string{"man/man1/bat.1"}
	if !reflect.DeepEqual(got.ManPages, wantMan) {
		t.Errorf("ManPages = %q, want %q", got.ManPages, wantMan)
	}
	wantComp := []string{"autocomplete/_bat", "autocomplete/bat.bash", "autocomplete/bat.fish"}
	if !reflect.DeepEqual(got.Completions, wantComp) {
		t.Errorf("Completions = %q, want %q", got.Completions, wantComp)
	}
}

func TestMatchBinName(t *testing.T) {
	l := &Layout{Executables: []string{"bat", "subdir/bat-helper"}}
	if got := l.MatchBinName("bat"); got != "bat" {
		t.Errorf("MatchBinName(bat) = %q, want %q", got, "bat")
	}
	if got := l.MatchBinName("missing"); got != "" {
		t.Errorf("MatchBinName(missing) = %q, want empty", got)
	}

	// Windows .exe variant.
	l2 := &Layout{Executables: []string{"bat.exe"}}
	if got := l2.MatchBinName("bat"); got != "bat.exe" {
		t.Errorf("MatchBinName(bat) on .exe = %q, want %q", got, "bat.exe")
	}
}
