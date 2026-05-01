package mcp

import (
	"context"
	"errors"
	"strings"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/disbug-io/disbug-cli/internal/client"
	"github.com/disbug-io/disbug-cli/internal/errfmt"
)

// SearchSessionsInput is the input for the search_sessions MCP tool.
type SearchSessionsInput struct {
	Query string `json:"query" jsonschema:"Search query"`
	Scope string `json:"scope,omitempty" jsonschema:"Search scope: sessions or pins; defaults to sessions"`
	Limit int    `json:"limit,omitempty" jsonschema:"Maximum results to return; defaults to 20 and is capped at 50"`
}

func registerSearchSessions(srv *sdkmcp.Server, deps *Deps) {
	sdkmcp.AddTool[SearchSessionsInput, client.SearchSessionsResponse](srv, &sdkmcp.Tool{
		Name: "search_sessions",
		Description: "Full-text search over session metadata and pin feedback within the team. " +
			"Returns lightweight session summaries, ranked by relevance. For listing by status/project, use list_sessions.",
	}, func(
		ctx context.Context,
		_ *sdkmcp.CallToolRequest,
		in SearchSessionsInput,
	) (*sdkmcp.CallToolResult, client.SearchSessionsResponse, error) {
		if deps == nil || deps.Client == nil {
			return nil, client.SearchSessionsResponse{}, errors.New("disbug API client is not configured")
		}
		if strings.TrimSpace(in.Query) == "" {
			return nil, client.SearchSessionsResponse{}, errors.New(errfmt.Format(&errfmt.UsageError{
				Message: "query is required",
			}))
		}

		scope := in.Scope
		if scope == "" {
			scope = "sessions"
		}
		if scope != "sessions" && scope != "pins" {
			return nil, client.SearchSessionsResponse{}, errors.New(errfmt.Format(&errfmt.UsageError{
				Message: "scope must be sessions or pins",
			}))
		}

		limit, err := searchLimit(in.Limit)
		if err != nil {
			return nil, client.SearchSessionsResponse{}, errors.New(errfmt.Format(err))
		}

		if err := deps.Client.RequireCapability(ctx, "search"); err != nil {
			return nil, client.SearchSessionsResponse{}, errors.New(errfmt.Format(err))
		}

		resp, err := deps.Client.SearchSessions(ctx, &client.SearchParams{
			Query: in.Query,
			Scope: scope,
			Limit: limit,
		})
		if err != nil {
			return nil, client.SearchSessionsResponse{}, errors.New(errfmt.Format(err))
		}
		if resp == nil {
			return nil, client.SearchSessionsResponse{}, errors.New("disbug API returned no search sessions")
		}

		return jsonResult(resp), *resp, nil
	})
}

func searchLimit(limit int) (int, error) {
	if limit < 0 {
		return 0, &errfmt.UsageError{Message: "limit must be greater than or equal to 0"}
	}
	if limit == 0 {
		return 20, nil
	}
	if limit > 50 {
		return 50, nil
	}

	return limit, nil
}
