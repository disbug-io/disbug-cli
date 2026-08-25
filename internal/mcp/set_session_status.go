package mcp

import (
	"context"
	"errors"
	"strings"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/disbug-io/disbug-cli/internal/errfmt"
	"github.com/disbug-io/disbug-cli/internal/ref"
)

// SetSessionStatusInput is the input for the set_session_status MCP tool.
type SetSessionStatusInput struct {
	Target string `json:"target" jsonschema:"Disbug session report URL"`
	Status string `json:"status" jsonschema:"New status: open, resolved, or dismissed"`
	Note   string `json:"note,omitempty" jsonschema:"Optional agent activity note"`
}

func registerSetSessionStatus(srv *sdkmcp.Server, deps *Deps) {
	sdkmcp.AddTool[SetSessionStatusInput, Result](srv, &sdkmcp.Tool{
		Name:        "set_session_status",
		Description: "Update a Disbug session status and optionally record an agent note.",
	}, func(
		ctx context.Context,
		_ *sdkmcp.CallToolRequest,
		in SetSessionStatusInput,
	) (*sdkmcp.CallToolResult, Result, error) {
		if err := requireCloud(deps); err != nil {
			return nil, nil, toolErr(err)
		}
		if err := validateStatus(in.Status); err != nil {
			return nil, nil, toolErr(err)
		}

		sessionRef, err := ref.ParseSession(strings.TrimSpace(in.Target))
		if err != nil {
			return nil, nil, errors.New(errfmt.Format(&errfmt.UsageError{Message: err.Error()}))
		}
		if err := deps.Client.RequireCapability(ctx, "status_updates"); err != nil {
			return nil, nil, toolErr(err)
		}

		resp, err := deps.Client.SetSessionStatus(ctx, sessionRef, in.Status, strings.TrimSpace(in.Note))
		if err != nil {
			return nil, nil, toolErr(err)
		}
		if resp == nil {
			return nil, nil, errors.New("disbug API returned no session")
		}

		result := resultFrom(resp)
		return jsonResult(result), result, nil
	})
}
