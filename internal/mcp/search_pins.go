package mcp

import (
	"context"
	"errors"
	"strings"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/disbug-io/disbug-cli/internal/client"
	"github.com/disbug-io/disbug-cli/internal/errfmt"
)

// SearchPinsInput is the input for the search_pins MCP tool.
type SearchPinsInput struct {
	Query string `json:"query" jsonschema:"Search query"`
	Limit int    `json:"limit,omitempty" jsonschema:"Maximum results to return; defaults to 20 and is capped at 50"`
}

func registerSearchPins(srv *sdkmcp.Server, deps *Deps) {
	sdkmcp.AddTool[SearchPinsInput, client.SearchPinsResponse](srv, &sdkmcp.Tool{
		Name:        "search_pins",
		Description: "Full-text search over pin feedback within the team, returning matching pins, not session summaries.",
	}, func(
		ctx context.Context,
		_ *sdkmcp.CallToolRequest,
		in SearchPinsInput,
	) (*sdkmcp.CallToolResult, client.SearchPinsResponse, error) {
		if deps == nil || deps.Client == nil {
			return nil, client.SearchPinsResponse{}, errors.New("disbug API client is not configured")
		}
		if strings.TrimSpace(in.Query) == "" {
			return nil, client.SearchPinsResponse{}, errors.New(errfmt.Format(&errfmt.UsageError{
				Message: "query is required",
			}))
		}

		limit, err := searchLimit(in.Limit)
		if err != nil {
			return nil, client.SearchPinsResponse{}, errors.New(errfmt.Format(err))
		}

		if err := deps.Client.RequireCapability(ctx, "search"); err != nil {
			return nil, client.SearchPinsResponse{}, errors.New(errfmt.Format(err))
		}

		resp, err := deps.Client.SearchPins(ctx, &client.SearchParams{
			Query: in.Query,
			Scope: "pins",
			Limit: limit,
		})
		if err != nil {
			return nil, client.SearchPinsResponse{}, errors.New(errfmt.Format(err))
		}
		if resp == nil {
			return nil, client.SearchPinsResponse{}, errors.New("disbug API returned no search pins")
		}

		return jsonResult(resp), *resp, nil
	})
}
