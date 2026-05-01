package cmd

import (
	"context"
	"strings"

	"github.com/disbug-io/disbug-cli/internal/client"
	"github.com/disbug-io/disbug-cli/internal/errfmt"
	"github.com/disbug-io/disbug-cli/internal/outfmt"
)

// SearchCmd searches sessions or pins.
type SearchCmd struct {
	Query string `arg:"" name:"query" help:"Search query"`
	Scope string `help:"Search scope" enum:"sessions,pins" default:"sessions"`
	Limit int    `help:"Max results (1-50)" default:"20"`
}

// Run searches Disbug data and writes the response as JSON.
func (c *SearchCmd) Run(ctx context.Context, b bindings) error {
	if c.Limit < 1 || c.Limit > 50 {
		return &errfmt.UsageError{Message: "--limit must be between 1 and 50"}
	}
	if strings.TrimSpace(c.Query) == "" {
		return &errfmt.UsageError{Message: "query is required"}
	}
	if c.Scope != "sessions" && c.Scope != "pins" {
		return &errfmt.UsageError{Message: "unsupported scope"}
	}

	cli, _, err := newAuthenticatedClient(b.Flags)
	if err != nil {
		return err
	}

	if err := cli.RequireCapability(ctx, "search"); err != nil {
		return err
	}

	params := &client.SearchParams{
		Query: c.Query,
		Scope: c.Scope,
		Limit: c.Limit,
	}
	switch c.Scope {
	case "sessions":
		resp, err := cli.SearchSessions(ctx, params)
		if err != nil {
			return err
		}
		return outfmt.WriteJSON(b.Stdout, resp, b.Flags.Pretty)
	case "pins":
		resp, err := cli.SearchPins(ctx, params)
		if err != nil {
			return err
		}
		return outfmt.WriteJSON(b.Stdout, resp, b.Flags.Pretty)
	}

	return nil
}
