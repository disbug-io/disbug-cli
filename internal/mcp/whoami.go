package mcp

import (
	"context"
	"errors"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/disbug-io/disbug-cli/internal/client"
)

// WhoamiInput is the empty input for the whoami MCP tool.
type WhoamiInput struct{}

func registerWhoami(srv *sdkmcp.Server, deps *Deps) {
	sdkmcp.AddTool[WhoamiInput, client.Me](srv, &sdkmcp.Tool{
		Name:        "whoami",
		Description: "Verify the active Disbug agent token and return the associated agent and team identity.",
	}, func(ctx context.Context, _ *sdkmcp.CallToolRequest, _ WhoamiInput) (*sdkmcp.CallToolResult, client.Me, error) {
		if deps == nil || deps.Client == nil {
			return errResult(errors.New("disbug API client is not configured")), client.Me{}, nil
		}

		me, err := deps.Client.Me(ctx)
		if err != nil {
			return errResult(err), client.Me{}, nil
		}
		if me == nil {
			return errResult(errors.New("disbug API returned no identity")), client.Me{}, nil
		}

		return jsonResult(me), *me, nil
	})
}
