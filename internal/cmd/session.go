package cmd

import (
	"context"

	"github.com/disbug-io/disbug-cli/internal/errfmt"
	"github.com/disbug-io/disbug-cli/internal/outfmt"
	"github.com/disbug-io/disbug-cli/internal/ref"
)

// SessionCmd groups session read and status commands while preserving `disbug session <ref>`.
type SessionCmd struct {
	Get    SessionGetCmd    `cmd:"" default:"withargs" help:"Show a cloud session summary."`
	Status SessionStatusCmd `cmd:"" help:"Update a cloud session status."`
}

// SessionGetCmd shows a single session by report URL.
type SessionGetCmd struct {
	Ref string `arg:"" name:"url" help:"Disbug report URL (e.g. https://app.disbug.io/team/projects/1/sessions/2/)"`
}

// Run fetches a session detail response and writes it as JSON.
func (c *SessionGetCmd) Run(ctx context.Context, b bindings) error {
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

// SessionStatusCmd updates a session status with an optional agent-authored note.
type SessionStatusCmd struct {
	Ref    string `arg:"" name:"ref" help:"Disbug session report URL."`
	Status string `arg:"" name:"status" enum:"open,resolved,dismissed" help:"New session status."`
	Note   string `help:"Optional activity-log note."`
}

// Run updates the session and writes the updated detail as JSON.
func (c *SessionStatusCmd) Run(ctx context.Context, b bindings) error {
	sref, err := ref.ParseSession(c.Ref)
	if err != nil {
		return &errfmt.UsageError{Message: err.Error()}
	}

	cli, _, err := newAuthenticatedClient(b.Flags)
	if err != nil {
		return err
	}
	if err := cli.RequireCapability(ctx, "status_updates"); err != nil {
		return err
	}

	resp, err := cli.SetSessionStatus(ctx, sref, c.Status, c.Note)
	if err != nil {
		return err
	}

	return outfmt.WriteJSON(b.Stdout, resp, b.Flags.Pretty)
}
