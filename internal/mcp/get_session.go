package mcp

import (
	"context"
	"errors"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/disbug-io/disbug-cli/internal/client"
	"github.com/disbug-io/disbug-cli/internal/errfmt"
	"github.com/disbug-io/disbug-cli/internal/ref"
)

// GetSessionInput is the input for the get_session MCP tool.
type GetSessionInput struct {
	Session string `json:"session" jsonschema:"session id e.g. 7392"`
}

func registerGetSession(srv *sdkmcp.Server, deps *Deps) {
	sdkmcp.AddTool[GetSessionInput, client.SessionDetail](srv, &sdkmcp.Tool{
		Name:        "get_session",
		Description: "Get full details for a Disbug session, including pins.",
	}, func(
		ctx context.Context,
		_ *sdkmcp.CallToolRequest,
		in GetSessionInput,
	) (*sdkmcp.CallToolResult, client.SessionDetail, error) {
		if deps == nil || deps.Client == nil {
			return nil, client.SessionDetail{}, errors.New("disbug API client is not configured")
		}

		sessionRef, err := ref.ParseSession(in.Session)
		if err != nil {
			return nil, client.SessionDetail{}, errors.New(errfmt.Format(&errfmt.UsageError{Message: err.Error()}))
		}

		resp, err := deps.Client.GetSession(ctx, sessionRef.ID)
		if err != nil {
			return nil, client.SessionDetail{}, errors.New(errfmt.Format(err))
		}
		if resp == nil {
			return nil, client.SessionDetail{}, errors.New("disbug API returned no session")
		}

		return jsonResult(resp), *resp, nil
	})
}
