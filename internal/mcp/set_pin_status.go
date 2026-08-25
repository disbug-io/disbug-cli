package mcp

import (
	"context"
	"errors"
	"strings"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/disbug-io/disbug-cli/internal/errfmt"
	"github.com/disbug-io/disbug-cli/internal/ref"
)

// SetPinStatusInput is the input for the set_pin_status MCP tool.
type SetPinStatusInput struct {
	Target string `json:"target" jsonschema:"Disbug report URL with ?pin=<number>"`
	Status string `json:"status" jsonschema:"New status: open, resolved, or dismissed"`
	Note   string `json:"note,omitempty" jsonschema:"Optional agent activity note"`
}

func registerSetPinStatus(srv *sdkmcp.Server, deps *Deps) {
	sdkmcp.AddTool[SetPinStatusInput, Result](srv, &sdkmcp.Tool{
		Name:        "set_pin_status",
		Description: "Update a Disbug pin status and optionally record a pin-specific agent note.",
	}, func(
		ctx context.Context,
		_ *sdkmcp.CallToolRequest,
		in SetPinStatusInput,
	) (*sdkmcp.CallToolResult, Result, error) {
		if err := requireCloud(deps); err != nil {
			return nil, nil, toolErr(err)
		}
		if err := validateStatus(in.Status); err != nil {
			return nil, nil, toolErr(err)
		}

		pinRef, err := ref.ParsePin(strings.TrimSpace(in.Target))
		if err != nil {
			return nil, nil, errors.New(errfmt.Format(&errfmt.UsageError{Message: err.Error()}))
		}
		if err := deps.Client.RequireCapability(ctx, "status_updates"); err != nil {
			return nil, nil, toolErr(err)
		}

		resp, err := deps.Client.SetPinStatus(ctx, pinRef, in.Status, strings.TrimSpace(in.Note))
		if err != nil {
			return nil, nil, toolErr(err)
		}
		if resp == nil {
			return nil, nil, errors.New("disbug API returned no pin")
		}

		result := resultFrom(resp)
		return jsonResult(result), result, nil
	})
}

func validateStatus(status string) error {
	switch status {
	case "open", "resolved", "dismissed":
		return nil
	default:
		return &errfmt.UsageError{Message: "status must be open, resolved, or dismissed"}
	}
}
