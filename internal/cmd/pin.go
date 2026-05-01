package cmd

import (
	"context"
	"strings"

	"github.com/disbug-io/disbug-cli/internal/errfmt"
	"github.com/disbug-io/disbug-cli/internal/outfmt"
	"github.com/disbug-io/disbug-cli/internal/ref"
)

// PinCmd shows a single pin by session and pin number.
type PinCmd struct {
	Ref    string `arg:"" name:"pin" help:"Pin reference (e.g. 7392.2)"`
	Fields string `help:"Comma-separated fields: screenshot,console,network,events,replay,voice_note,video,all" default:"all"`
}

// Run fetches a pin detail response and writes it as JSON.
func (c *PinCmd) Run(ctx context.Context, b bindings) error {
	pinRef, err := ref.ParsePin(c.Ref)
	if err != nil {
		return &errfmt.UsageError{Message: err.Error()}
	}

	fields, err := ref.NormalizeFields(strings.Split(c.Fields, ","))
	if err != nil {
		return &errfmt.UsageError{Message: err.Error()}
	}

	cli, _, err := newAuthenticatedClient(b.Flags)
	if err != nil {
		return err
	}

	if err := cli.RequireCapability(ctx, "pin_field_selection"); err != nil {
		return err
	}
	if err := cli.RequireCapability(ctx, "pin_by_number"); err != nil {
		return err
	}

	resp, err := cli.GetPinByNumber(ctx, pinRef.Session, pinRef.Pin, fields)
	if err != nil {
		return err
	}

	return outfmt.WriteJSON(b.Stdout, resp, b.Flags.Pretty)
}
