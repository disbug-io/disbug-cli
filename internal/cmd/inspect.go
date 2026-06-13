package cmd

import (
	"context"
	"strings"

	"github.com/disbug-io/disbug-cli/internal/errfmt"
	"github.com/disbug-io/disbug-cli/internal/localbundle"
	"github.com/disbug-io/disbug-cli/internal/outfmt"
	"github.com/disbug-io/disbug-cli/internal/ref"
)

// InspectCmd reads a downloaded local Disbug report JSON file.
type InspectCmd struct {
	Path     string `arg:"" name:"path" help:"Path to a downloaded Disbug local report JSON file."`
	Pin      int    `help:"Inspect one pin by number instead of returning the session summary."`
	Fields   string `help:"Comma-separated fields for --pin: screenshot,console,network,events,replay,voice_note,video,all."`
	CacheDir string `name:"cache-dir" help:"Directory for decoded local artifacts. Defaults to the user cache directory."`
}

type inspectPinOutput struct {
	Source string                 `json:"source"`
	Path   string                 `json:"path"`
	Pin    localbundle.PinInspect `json:"pin"`
}

// Run prints compact local report data, decoding heavy artifacts only when requested.
func (c *InspectCmd) Run(_ context.Context, b bindings) error {
	bundle, err := localbundle.Load(c.Path)
	if err != nil {
		return &errfmt.UsageError{Message: err.Error()}
	}

	if c.Pin <= 0 {
		summary, err := bundle.Summary()
		if err != nil {
			return &errfmt.UsageError{Message: err.Error()}
		}
		return outfmt.WriteJSON(b.Stdout, summary, b.Flags.Pretty)
	}

	fields := []string{}
	if strings.TrimSpace(c.Fields) != "" {
		var err error
		fields, err = ref.NormalizeFields(strings.Split(c.Fields, ","))
		if err != nil {
			return &errfmt.UsageError{Message: err.Error()}
		}
	}

	pin, err := bundle.InspectPin(c.Pin, fields, c.CacheDir)
	if err != nil {
		return &errfmt.UsageError{Message: err.Error()}
	}
	return outfmt.WriteJSON(b.Stdout, inspectPinOutput{
		Source: "local",
		Path:   bundle.Path,
		Pin:    pin,
	}, b.Flags.Pretty)
}
