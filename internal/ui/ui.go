package ui

import (
	"fmt"
	"io"
	"strings"
)

// ColorMode controls whether CLI output should use terminal colors.
type ColorMode int

const (
	// ColorAuto enables color only when output is connected to a TTY.
	ColorAuto ColorMode = iota
	// ColorAlways always enables color.
	ColorAlways
	// ColorNever always disables color.
	ColorNever
)

// ParseColorMode parses a user-provided color mode.
func ParseColorMode(value string) (ColorMode, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "auto":
		return ColorAuto, nil
	case "always":
		return ColorAlways, nil
	case "never":
		return ColorNever, nil
	default:
		return ColorAuto, fmt.Errorf("invalid color mode %q: expected auto, always, or never", value)
	}
}

func colorEnabled(mode ColorMode, isTTY bool) bool {
	switch mode {
	case ColorAlways:
		return true
	case ColorNever:
		return false
	default:
		return isTTY
	}
}

// UI owns CLI output streams and color capability for rendering user-facing output.
type UI struct {
	stdout io.Writer
	stderr io.Writer
	color  bool
}

// New creates a UI with resolved color support.
func New(stdout, stderr io.Writer, mode ColorMode, isTTY bool) *UI {
	return &UI{
		stdout: stdout,
		stderr: stderr,
		color:  colorEnabled(mode, isTTY),
	}
}

// Stdout returns the configured standard output writer.
func (u *UI) Stdout() io.Writer {
	return u.stdout
}

// Stderr returns the configured standard error writer.
func (u *UI) Stderr() io.Writer {
	return u.stderr
}

// Color reports whether color output is enabled.
func (u *UI) Color() bool {
	return u.color
}

// Outf writes formatted output to stdout.
func (u *UI) Outf(format string, a ...any) {
	_, _ = fmt.Fprintf(u.stdout, format, a...)
}

// Errf writes formatted output to stderr.
func (u *UI) Errf(format string, a ...any) {
	_, _ = fmt.Fprintf(u.stderr, format, a...)
}

// Errln writes a line to stderr.
func (u *UI) Errln(args ...any) {
	_, _ = fmt.Fprintln(u.stderr, args...)
}
