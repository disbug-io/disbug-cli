package cmd

import (
	"context"
	"strings"

	"github.com/disbug-io/disbug-cli/internal/errfmt"
	"github.com/disbug-io/disbug-cli/internal/outfmt"
	"github.com/disbug-io/disbug-cli/internal/ref"
)

// PinCmd shows a single pin by report URL and pin number.
type PinCmd struct {
	Ref    string `arg:"" name:"url" help:"Disbug report URL with ?pin=<number>."`
	Pin    int64  `help:"Pin number when the report URL does not include ?pin=."`
	Fields string `help:"Comma-separated fields: screenshot,console,network,events,replay,voice_note,video,all" default:"all"`
}

// Run fetches a pin detail response and writes it as JSON.
func (c *PinCmd) Run(ctx context.Context, b bindings) error {
	pinRef, err := ref.ParsePin(c.Ref)
	if err != nil {
		sessionRef, sessionErr := ref.ParseSession(c.Ref)
		if sessionErr != nil || c.Pin <= 0 {
			return &errfmt.UsageError{Message: err.Error()}
		}
		pinRef = ref.PinRef{Session: sessionRef, Pin: c.Pin}
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
	if err := cli.RequireCapability(ctx, "scoped_pin_lookup"); err != nil {
		return err
	}

	resp, err := cli.GetPinByNumber(ctx, pinRef.Session, pinRef.Pin, fields)
	if err != nil {
		return err
	}

	resolved, err := cli.ResolveReplay(ctx, resp, pinRef.Session.SessionNumber, pinRef.Pin)
	if err != nil {
		return err
	}

	return outfmt.WriteJSON(b.Stdout, resolved, b.Flags.Pretty)
}
