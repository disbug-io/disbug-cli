package mcp

import (
	"context"
	"errors"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/disbug-io/disbug-cli/internal/client"
	"github.com/disbug-io/disbug-cli/internal/errfmt"
)

// ListSessionsInput is the input for the list_sessions MCP tool.
type ListSessionsInput struct {
	Status  string `json:"status,omitempty" jsonschema:"Filter by status: open, resolved, or dismissed"`
	Project string `json:"project,omitempty" jsonschema:"Filter by project slug"`
	Limit   int    `json:"limit,omitempty" jsonschema:"Maximum results to return; defaults to 50 and is capped at 100"`
}

func registerListSessions(srv *sdkmcp.Server, deps *Deps) {
	sdkmcp.AddTool[ListSessionsInput, client.ListSessionsResponse](srv, &sdkmcp.Tool{
		Name:        "list_sessions",
		Description: "List Disbug sessions with optional status and project filters.",
	}, func(
		ctx context.Context,
		_ *sdkmcp.CallToolRequest,
		in ListSessionsInput,
	) (*sdkmcp.CallToolResult, client.ListSessionsResponse, error) {
		if deps == nil || deps.Client == nil {
			return nil, client.ListSessionsResponse{}, errors.New("disbug API client is not configured")
		}

		limit, err := listSessionsLimit(in.Limit)
		if err != nil {
			return nil, client.ListSessionsResponse{}, errors.New(errfmt.Format(err))
		}

		resp, err := deps.Client.ListSessions(ctx, &client.ListSessionsParams{
			Status:  in.Status,
			Project: in.Project,
			Limit:   limit,
		})
		if err != nil {
			return nil, client.ListSessionsResponse{}, errors.New(errfmt.Format(err))
		}
		if resp == nil {
			return nil, client.ListSessionsResponse{}, errors.New("disbug API returned no sessions")
		}

		return jsonResult(resp), *resp, nil
	})
}

func listSessionsLimit(limit int) (int, error) {
	if limit < 0 {
		return 0, &errfmt.UsageError{Message: "--limit must be greater than or equal to 0"}
	}
	if limit == 0 {
		return 50, nil
	}
	if limit > 100 {
		return 100, nil
	}

	return limit, nil
}
