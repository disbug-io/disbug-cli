package cmd

import (
	"context"

	"github.com/disbug-io/disbug-cli/internal/outfmt"
)

// WhoamiCmd shows the current token identity.
type WhoamiCmd struct{}

// Run fetches the current identity and writes it as JSON.
func (c *WhoamiCmd) Run(ctx context.Context, b bindings) error {
	cli, _, err := newAuthenticatedClient(b.Flags)
	if err != nil {
		return err
	}

	me, err := cli.Me(ctx)
	if err != nil {
		return err
	}

	return outfmt.WriteJSON(b.Stdout, me, b.Flags.Pretty)
}
