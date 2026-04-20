package cmd

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/ascarter/gh-tool/internal/paths"
)

func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = orig })

	errCh := make(chan error, 1)
	go func() { errCh <- fn(); w.Close() }()
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String(), <-errCh
}

func TestRunShellOutputs(t *testing.T) {
	dirs := paths.Dirs{
		Config: "/x/config", Data: "/x/data", State: "/x/state", Cache: "/x/cache",
	}
	for _, shell := range []string{"bash", "zsh", "fish"} {
		t.Run(shell, func(t *testing.T) {
			out, err := captureStdout(t, func() error { return emitShell(dirs, shell, shellOptions{}) })
			if err != nil {
				t.Fatalf("err=%v", err)
			}
			if !strings.Contains(out, dirs.BinDir()) {
				t.Errorf("%s output missing BinDir; got:\n%s", shell, out)
			}
		})
	}
}

func TestRunShellUnknown(t *testing.T) {
	dirs := paths.Dirs{Data: "/x/data"}
	if err := emitShell(dirs, "tcsh", shellOptions{}); err == nil {
		t.Error("expected error for unsupported shell")
	}
}

func TestDetectShell(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"/bin/bash", "bash", false},
		{"/usr/bin/zsh", "zsh", false},
		{"/opt/homebrew/bin/fish", "fish", false},
		{"bash", "bash", false},
		{"/bin/tcsh", "", true},
		{"", "", true},
	}
	for _, tt := range tests {
		got, err := detectShell(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("detectShell(%q) = %q, want error", tt.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("detectShell(%q) error: %v", tt.in, err)
		}
		if got != tt.want {
			t.Errorf("detectShell(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestEmitShellNoCompletions(t *testing.T) {
	dirs := paths.Dirs{Data: "/x/data"}
	for _, shell := range []string{"bash", "zsh", "fish"} {
		t.Run(shell, func(t *testing.T) {
			out, err := captureStdout(t, func() error {
				return emitShell(dirs, shell, shellOptions{NoCompletions: true})
			})
			if err != nil {
				t.Fatalf("err=%v", err)
			}
			if strings.Contains(strings.ToLower(out), "completion") {
				t.Errorf("%s with --no-completions should not mention completions; got:\n%s", shell, out)
			}
		})
	}
}

func TestEmitShellInteractiveGuards(t *testing.T) {
	dirs := paths.Dirs{Data: "/x/data"}
	cases := map[string]string{
		"bash": "$- == *i*",
		"zsh":  "-o interactive",
		"fish": "status is-interactive",
	}
	for shell, marker := range cases {
		t.Run(shell, func(t *testing.T) {
			out, err := captureStdout(t, func() error { return emitShell(dirs, shell, shellOptions{}) })
			if err != nil {
				t.Fatalf("err=%v", err)
			}
			if !strings.Contains(out, marker) {
				t.Errorf("%s output missing interactive guard %q; got:\n%s", shell, marker, out)
			}
		})
	}
}

func TestEmitShellGhtoolHomeExport(t *testing.T) {
	dirs := paths.Dirs{Data: "/x/data"}
	home := "/opt/gh-tool"
	cases := map[string]string{
		"bash": `export GHTOOL_HOME="/opt/gh-tool"`,
		"zsh":  `export GHTOOL_HOME="/opt/gh-tool"`,
		"fish": `set -gx GHTOOL_HOME "/opt/gh-tool"`,
	}
	for shell, want := range cases {
		t.Run(shell, func(t *testing.T) {
			out, err := captureStdout(t, func() error {
				return emitShell(dirs, shell, shellOptions{GhtoolHome: home})
			})
			if err != nil {
				t.Fatalf("err=%v", err)
			}
			if !strings.Contains(out, want) {
				t.Errorf("%s output missing %q; got:\n%s", shell, want, out)
			}
		})
	}
}

func TestEmitShellNoGhtoolHomeWhenUnset(t *testing.T) {
	dirs := paths.Dirs{Data: "/x/data"}
	out, err := captureStdout(t, func() error { return emitShell(dirs, "bash", shellOptions{}) })
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if strings.Contains(out, "GHTOOL_HOME") {
		t.Errorf("output should not mention GHTOOL_HOME when unset; got:\n%s", out)
	}
}
