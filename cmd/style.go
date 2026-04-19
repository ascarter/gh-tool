package cmd

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

// stdoutIsTTY reports whether stdout is attached to a terminal that should
// receive ANSI styling. Cached on first call.
var stdoutIsTTY = func() bool {
	return term.IsTerminal(int(os.Stdout.Fd())) && os.Getenv("NO_COLOR") == ""
}()

const (
	ansiReset      = "\x1b[0m"
	ansiBold       = "\x1b[1m"
	ansiYellow     = "\x1b[33m"
	ansiBoldYellow = "\x1b[1;33m"
)

// warnf prints a warning line to stdout. The leading "⚠ Warning:" label is
// rendered in bold yellow on a TTY, plain text otherwise. The remaining
// message is printed unstyled. Trailing newline is added automatically.
func warnf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if stdoutIsTTY {
		fmt.Printf("%s⚠ Warning:%s %s\n", ansiBoldYellow, ansiReset, msg)
		return
	}
	fmt.Printf("Warning: %s\n", msg)
}
