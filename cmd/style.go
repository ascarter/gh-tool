package cmd

import (
	"fmt"

	"github.com/ascarter/gh-tool/internal/ui"
)

// warnf prints a warning line to stdout. The leading "⚠ Warning:" label is
// rendered in bold yellow on a TTY, plain text otherwise. Trailing newline
// is added automatically. Style is sourced from internal/ui so the warn
// output, the live install view, and the line reporter agree.
func warnf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Printf("%s %s\n", ui.WarnLabel(ui.IconWarn+" Warning:"), msg)
}
