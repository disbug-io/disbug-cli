package cmd

import (
	"fmt"

	"github.com/disbug-io/disbug-cli/internal/errfmt"
)

// Set at build time via -ldflags.
var (
	version = "dev"
	commit  = ""
	date    = ""
)

// VersionString returns the human-readable version string.
func VersionString() string {
	if commit != "" {
		return fmt.Sprintf("%s (%s, %s)", version, commit, date)
	}
	return version
}

// ExitCode returns the exit code for the given error.
func ExitCode(err error) int {
	return errfmt.ExitCode(err)
}
