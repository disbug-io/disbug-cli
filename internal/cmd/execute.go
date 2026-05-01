package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
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

// Execute parses args and dispatches. Stub for now — replaced in Task 3.1.
func Execute(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	return errors.New("not implemented")
}

// ExitCode returns the exit code for the given error. Stub — replaced in Task 1.4.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	return 1
}
