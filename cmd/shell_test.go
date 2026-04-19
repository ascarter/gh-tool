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
			out, err := captureStdout(t, func() error { return emitShell(dirs, shell) })
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
	if err := emitShell(dirs, "tcsh"); err == nil {
		t.Error("expected error for unsupported shell")
	}
}
