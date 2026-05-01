package cmd

import "fmt"

// CompletionCmd prints shell completion scripts.
type CompletionCmd struct {
	Shell string `arg:"" enum:"bash,zsh,fish,powershell" help:"Shell to generate completions for."`
}

// Run writes the placeholder shell completion script.
func (c CompletionCmd) Run(b bindings) error {
	_, err := fmt.Fprintf(b.Stdout, "# disbug %s completion (placeholder; implemented in Phase 6)\n", c.Shell)
	return err
}
