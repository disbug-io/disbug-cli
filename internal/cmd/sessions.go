package cmd

import (
	"context"

	"github.com/disbug-io/disbug-cli/internal/client"
	"github.com/disbug-io/disbug-cli/internal/errfmt"
	"github.com/disbug-io/disbug-cli/internal/outfmt"
)

// SessionsCmd lists sessions.
type SessionsCmd struct {
	Status  string `help:"Filter by status" enum:"open,resolved,dismissed," default:""`
	Project string `help:"Filter by project slug"`
	Limit   int    `help:"Max results (1-100)" default:"50"`
	Cursor  string `help:"Pagination cursor"`
}

// Run lists sessions and writes the paginated response as JSON.
func (c *SessionsCmd) Run(ctx context.Context, b bindings) error {
	if c.Limit < 1 || c.Limit > 100 {
		return &errfmt.UsageError{Message: "--limit must be between 1 and 100"}
	}

	cli, _, err := newAuthenticatedClient(b.Flags)
	if err != nil {
		return err
	}

	resp, err := cli.ListSessions(ctx, &client.ListSessionsParams{
		Status:  c.Status,
		Project: c.Project,
		Limit:   c.Limit,
		Cursor:  c.Cursor,
	})
	if err != nil {
		return err
	}

	return outfmt.WriteJSON(b.Stdout, resp, b.Flags.Pretty)
}
