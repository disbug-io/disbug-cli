package mcp

import (
	"context"
	"errors"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/disbug-io/disbug-cli/internal/client"
	"github.com/disbug-io/disbug-cli/internal/errfmt"
	"github.com/disbug-io/disbug-cli/internal/ref"
)

// GetPinInput is the input for the get_pin MCP tool.
type GetPinInput struct {
	Pin    string   `json:"pin" jsonschema:"pin reference e.g. 7392.2"`
	Fields []string `json:"fields,omitempty" jsonschema:"array of: screenshot console network events replay voice_note video all"`
}

func registerGetPin(srv *sdkmcp.Server, deps *Deps) {
	sdkmcp.AddTool[GetPinInput, client.PinFull](srv, &sdkmcp.Tool{
		Name:        "get_pin",
		Description: "Get a Disbug pin by session.pin reference, optionally selecting included fields.",
	}, func(
		ctx context.Context,
		_ *sdkmcp.CallToolRequest,
		in GetPinInput,
	) (*sdkmcp.CallToolResult, client.PinFull, error) {
		if deps == nil || deps.Client == nil {
			return nil, client.PinFull{}, errors.New("disbug API client is not configured")
		}

		pinRef, err := ref.ParsePin(in.Pin)
		if err != nil {
			return nil, client.PinFull{}, errors.New(errfmt.Format(&errfmt.UsageError{Message: err.Error()}))
		}

		fields := in.Fields
		if len(fields) == 0 {
			fields = []string{"all"}
		}
		fields, err = ref.NormalizeFields(fields)
		if err != nil {
			return nil, client.PinFull{}, errors.New(errfmt.Format(&errfmt.UsageError{Message: err.Error()}))
		}

		if err := deps.Client.RequireCapability(ctx, "pin_field_selection"); err != nil {
			return nil, client.PinFull{}, errors.New(errfmt.Format(err))
		}
		if err := deps.Client.RequireCapability(ctx, "pin_by_number"); err != nil {
			return nil, client.PinFull{}, errors.New(errfmt.Format(err))
		}

		resp, err := deps.Client.GetPinByNumber(ctx, pinRef.Session, pinRef.Pin, fields)
		if err != nil {
			return nil, client.PinFull{}, errors.New(errfmt.Format(err))
		}
		if resp == nil {
			return nil, client.PinFull{}, errors.New("disbug API returned no pin")
		}

		return jsonResult(resp), *resp, nil
	})
}
