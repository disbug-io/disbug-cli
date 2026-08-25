package cmd

import (
	"context"
	"strings"

	"github.com/disbug-io/disbug-cli/internal/errfmt"
	"github.com/disbug-io/disbug-cli/internal/outfmt"
	"github.com/disbug-io/disbug-cli/internal/ref"
)

// PinCmd groups pin read and status commands while preserving `disbug pin <ref>`.
type PinCmd struct {
	Get    PinGetCmd    `cmd:"" default:"withargs" help:"Fetch selected evidence for one pin."`
	Status PinStatusCmd `cmd:"" help:"Update a cloud pin status."`
}

// PinGetCmd shows a single pin by report URL and pin number.
type PinGetCmd struct {
	Ref    string `arg:"" name:"url" help:"Disbug report URL with ?pin=<number>."`
	Pin    int64  `help:"Pin number when the report URL does not include ?pin=."`
	Fields string `help:"Comma-separated fields: screenshot,console,network,events,replay,voice_note,video,all" default:"all"`
}

// Run fetches a pin detail response and writes it as JSON.
func (c *PinGetCmd) Run(ctx context.Context, b bindings) error {
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

// PinStatusCmd updates a pin status with an optional agent-authored note.
type PinStatusCmd struct {
	Ref    string `arg:"" name:"ref" help:"Disbug report URL with ?pin=<number>."`
	Status string `arg:"" name:"status" enum:"open,resolved,dismissed" help:"New pin status."`
	Note   string `help:"Optional activity-log note."`
}

// Run updates the pin and writes the updated detail as JSON.
func (c *PinStatusCmd) Run(ctx context.Context, b bindings) error {
	pinRef, err := ref.ParsePin(c.Ref)
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

	resp, err := cli.SetPinStatus(ctx, pinRef, c.Status, c.Note)
	if err != nil {
		return err
	}

	return outfmt.WriteJSON(b.Stdout, resp, b.Flags.Pretty)
}
