package mcp

import (
	"context"
	"errors"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/disbug-io/disbug-cli/internal/client"
	"github.com/disbug-io/disbug-cli/internal/errfmt"
)

// WhoamiInput is the empty input for the whoami MCP tool.
type WhoamiInput struct{}

func registerWhoami(srv *sdkmcp.Server, deps *Deps) {
	sdkmcp.AddTool[WhoamiInput, client.Me](srv, &sdkmcp.Tool{
		Name:        "whoami",
		Description: "Verify the active Disbug agent token and return the associated agent and team identity.",
	}, func(ctx context.Context, _ *sdkmcp.CallToolRequest, _ WhoamiInput) (*sdkmcp.CallToolResult, client.Me, error) {
		if deps == nil || deps.Client == nil {
			return nil, client.Me{}, errors.New("disbug API client is not configured")
		}

		me, err := deps.Client.Me(ctx)
		if err != nil {
			return nil, client.Me{}, errors.New(errfmt.Format(err))
		}
		if me == nil {
			return nil, client.Me{}, errors.New("disbug API returned no identity")
		}

		return jsonResult(me), *me, nil
	})
}
