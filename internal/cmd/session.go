package cmd

import (
	"context"

	"github.com/disbug-io/disbug-cli/internal/errfmt"
	"github.com/disbug-io/disbug-cli/internal/outfmt"
	"github.com/disbug-io/disbug-cli/internal/ref"
)

// SessionCmd shows a single session by session URL.
type SessionCmd struct {
	Ref string `arg:"" name:"url" help:"Disbug session URL (e.g. https://app.disbug.io/team/projects/1/sessions/2/)"`
}

// Run fetches a session detail response and writes it as JSON.
func (c *SessionCmd) Run(ctx context.Context, b bindings) error {
	sref, err := ref.ParseSession(c.Ref)
	if err != nil {
		return &errfmt.UsageError{Message: err.Error()}
	}

	cli, _, err := newAuthenticatedClient(b.Flags)
	if err != nil {
		return err
	}

	resp, err := cli.GetSession(ctx, sref)
	if err != nil {
		return err
	}

	return outfmt.WriteJSON(b.Stdout, resp, b.Flags.Pretty)
}
