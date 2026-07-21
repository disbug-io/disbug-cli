package cmd

import (
	"context"
	"strings"

	"github.com/disbug-io/disbug-cli/internal/errfmt"
	"github.com/disbug-io/disbug-cli/internal/outfmt"
	"github.com/disbug-io/disbug-cli/internal/ref"
)

// ResolveCmd marks a session resolved and records what was fixed.
type ResolveCmd struct {
	Ref     string `arg:"" name:"url" help:"Disbug session URL (e.g. https://app.disbug.io/team/projects/1/sessions/2/)"`
	Summary string `help:"What changed and how the fix was verified." required:""`
}

// Run resolves a session and writes the updated session as JSON.
func (c *ResolveCmd) Run(ctx context.Context, b bindings) error {
	summary := strings.TrimSpace(c.Summary)
	if summary == "" {
		return &errfmt.UsageError{Message: "--summary must not be empty"}
	}

	sessionRef, err := ref.ParseSession(c.Ref)
	if err != nil {
		return &errfmt.UsageError{Message: err.Error()}
	}

	cli, _, err := newAuthenticatedClient(b.Flags)
	if err != nil {
		return err
	}
	if err := cli.RequireCapability(ctx, "resolve_session"); err != nil {
		return err
	}

	resp, err := cli.ResolveSession(ctx, sessionRef, summary)
	if err != nil {
		return err
	}

	return outfmt.WriteJSON(b.Stdout, resp, b.Flags.Pretty)
}
