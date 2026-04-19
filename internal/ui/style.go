// Package ui holds shared rendering primitives used across cmd/ — lipgloss
// style tokens, a small concurrent worker pool, and the line/live progress
// reporters used by install / upgrade / list.
//
// Splitting these out of cmd/ keeps the styling layer testable and lets the
// non-TTY line reporter and the TTY live reporter agree on icons and colors.
package ui

import (
	"os"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

// IsTTY reports whether stdout is attached to a terminal that should
// receive ANSI styling. NO_COLOR disables styling per the de-facto standard.
func IsTTY() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// Style tokens. Render returns the styled string on a TTY, the plain string
// otherwise — callers do not need to branch on IsTTY themselves.
var (
	styleSuccess = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))  // green
	styleWarn    = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))  // yellow
	styleError   = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))  // red
	styleMuted   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))  // bright black / gray
	styleBold    = lipgloss.NewStyle().Bold(true)
	styleWarnLbl = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("3"))
)

// Render functions. Each returns the styled string only when stdout is a
// TTY; otherwise the input is passed through unchanged.

func Success(s string) string   { return renderIf(styleSuccess, s) }
func Warn(s string) string      { return renderIf(styleWarn, s) }
func Error(s string) string     { return renderIf(styleError, s) }
func Muted(s string) string     { return renderIf(styleMuted, s) }
func Bold(s string) string      { return renderIf(styleBold, s) }
func WarnLabel(s string) string { return renderIf(styleWarnLbl, s) }

func renderIf(style lipgloss.Style, s string) string {
	if !IsTTY() {
		return s
	}
	return style.Render(s)
}

// Icons used across reporters. Plain ASCII fallbacks keep the line reporter
// usable when piped to a file or non-UTF8 sink.
const (
	IconSuccess = "✓"
	IconFailure = "✗"
	IconWarn    = "⚠"
	IconBullet  = "·"
	IconArrow   = "→"
)
