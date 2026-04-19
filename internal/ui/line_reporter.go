package ui

import (
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/ascarter/gh-tool/internal/tool"
)

// LineReporter implements tool.Reporter by emitting one line per event. It
// serialises writes through a mutex so concurrent calls (from a parallel
// install batch) don't interleave a single line. When Prefixed is true,
// every event line is prefixed with "[name] " — useful when multiple tools
// share stdout.
//
// In Prefixed (parallel) mode, warnings are buffered per tool and flushed
// together with the terminal Done/Fail line so each tool's output stays
// grouped in the scrollback. In unprefixed mode, warnings print immediately
// (preserves the historical single-tool UX where there's no interleaving
// to worry about).
//
// When Verbose is false (the default for batches), per-stage progress
// chatter ("Downloading…", "Verifying…", "Extracting…") is suppressed and
// only Warn/Done/Fail surface. Set Verbose to restore the full step log.
type LineReporter struct {
	W        io.Writer
	Prefixed bool
	Verbose  bool
	mu       sync.Mutex
	warns    map[string][]string
}

// NewLineReporter returns a LineReporter writing to stdout. When parallel
// is true, lines are tagged with their tool name and warnings are batched
// with their tool's terminal line. When verbose is true, per-stage progress
// messages are also printed.
func NewLineReporter(parallel, verbose bool) *LineReporter {
	return &LineReporter{W: os.Stdout, Prefixed: parallel, Verbose: verbose, warns: map[string][]string{}}
}

var _ tool.Reporter = (*LineReporter)(nil)

func (r *LineReporter) printf(name, format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	msg := fmt.Sprintf(format, args...)
	if r.Prefixed && name != "" {
		fmt.Fprintf(r.W, "[%s] %s\n", name, msg)
		return
	}
	fmt.Fprintln(r.W, msg)
}

// flushWarnsLocked writes any buffered warnings for name and clears them.
// Caller must hold r.mu.
func (r *LineReporter) flushWarnsLocked(name string) {
	ws, ok := r.warns[name]
	if !ok || len(ws) == 0 {
		return
	}
	label := WarnLabel(IconWarn + " Warning:")
	for _, w := range ws {
		if r.Prefixed && name != "" {
			fmt.Fprintf(r.W, "[%s] %s %s\n", name, label, w)
		} else {
			fmt.Fprintf(r.W, "%s %s\n", label, w)
		}
	}
	delete(r.warns, name)
}

func (r *LineReporter) Start(name string) {
	if !r.Prefixed || !r.Verbose {
		return
	}
	r.printf(name, "%s starting", IconBullet)
}

func (r *LineReporter) Stage(name, msg string) {
	if !r.Verbose {
		return
	}
	r.printf(name, "%s...", msg)
}

func (r *LineReporter) Warn(name, msg string) {
	if !r.Prefixed {
		// Single-tool path: print immediately, matches historical UX.
		label := WarnLabel(IconWarn + " Warning:")
		r.printf(name, "%s %s", label, msg)
		return
	}
	r.mu.Lock()
	if r.warns == nil {
		r.warns = map[string][]string{}
	}
	r.warns[name] = append(r.warns[name], msg)
	r.mu.Unlock()
}

func (r *LineReporter) Done(name, tag string) {
	r.mu.Lock()
	r.flushWarnsLocked(name)
	icon := Success(IconSuccess)
	var line string
	if tag != "" {
		line = fmt.Sprintf("%s Installed %s (%s)", icon, name, tag)
	} else {
		line = fmt.Sprintf("%s Done %s", icon, name)
	}
	if r.Prefixed && name != "" {
		fmt.Fprintf(r.W, "[%s] %s\n", name, line)
	} else {
		fmt.Fprintln(r.W, line)
	}
	r.mu.Unlock()
}

func (r *LineReporter) Fail(name string, err error) {
	r.mu.Lock()
	r.flushWarnsLocked(name)
	icon := Error(IconFailure)
	line := fmt.Sprintf("%s %s: %s", icon, name, err)
	if r.Prefixed && name != "" {
		fmt.Fprintf(r.W, "[%s] %s\n", name, line)
	} else {
		fmt.Fprintln(r.W, line)
	}
	r.mu.Unlock()
}
