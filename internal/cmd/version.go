package cmd

import "fmt"

// VersionCmd prints the CLI version.
type VersionCmd struct{}

// Run writes the version string.
func (VersionCmd) Run(b bindings) error {
	_, err := fmt.Fprintln(b.Stdout, VersionString())
	return err
}
