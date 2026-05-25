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
	Query  string `json:"query" jsonschema:"Search query"`
	Source string `json:"source,omitempty" jsonschema:"Source: auto, cloud, or local"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum results to return; defaults to 20 and is capped at 50"`
}

func registerSearchPins(srv *sdkmcp.Server, deps *Deps) {
	sdkmcp.AddTool[SearchPinsInput, Result](srv, &sdkmcp.Tool{
		Name:        "search_pins",
		Description: "Full-text search over pin feedback within the team, returning matching pins, not session summaries.",
	}, func(
		ctx context.Context,
		_ *sdkmcp.CallToolRequest,
		in SearchPinsInput,
	) (*sdkmcp.CallToolResult, Result, error) {
		if strings.TrimSpace(in.Query) == "" {
			return nil, nil, errors.New(errfmt.Format(&errfmt.UsageError{
				Message: "query is required",
			}))
		}
		source, err := normalizeSource(in.Source)
		if err != nil {
			return nil, nil, toolErr(err)
		}
		if source == sourceLocal || (source == sourceAuto && deps != nil && !deps.CloudAvailable && deps.LocalStore != nil) {
			store, err := requireLocal(deps)
			if err != nil {
				return nil, nil, errors.New(err.Error())
			}
			resp, err := store.SearchPins(ctx, in.Query, in.Limit)
			if err != nil {
				return nil, nil, errors.New(err.Error())
			}
			result := resultFrom(resp)
			return jsonResult(result), result, nil
		}
		if err := requireCloud(deps); err != nil {
			return nil, nil, toolErr(err)
		}

		limit, err := searchLimit(in.Limit)
		if err != nil {
			return nil, nil, errors.New(errfmt.Format(err))
		}

		if err := deps.Client.RequireCapability(ctx, "search"); err != nil {
			return nil, nil, errors.New("Pin search requires Disbug API capability \"search\"; your team's instance does not advertise it. Local fallback is currently only available for scope \"sessions\".")
		}

		resp, err := deps.Client.SearchPins(ctx, &client.SearchParams{
			Query: in.Query,
			Scope: "pins",
			Limit: limit,
		})
		if err != nil {
			return nil, nil, errors.New(errfmt.Format(err))
		}
		if resp == nil {
			return nil, nil, errors.New("disbug API returned no search pins")
		}

		result := resultFrom(resp)
		return jsonResult(result), result, nil
	})
}
