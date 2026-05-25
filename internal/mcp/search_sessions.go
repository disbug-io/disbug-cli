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
	Query  string `json:"query" jsonschema:"Search query"`
	Source string `json:"source,omitempty" jsonschema:"Source: auto, cloud, or local"`
	Scope  string `json:"scope,omitempty" jsonschema:"Search scope: sessions or pins; defaults to sessions"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum results to return; defaults to 20 and is capped at 50"`
}

func registerSearchSessions(srv *sdkmcp.Server, deps *Deps) {
	sdkmcp.AddTool[SearchSessionsInput, Result](srv, &sdkmcp.Tool{
		Name: "search_sessions",
		Description: "Full-text search over session metadata and pin feedback within the team. " +
			"Returns lightweight session summaries, ranked by relevance. For listing by status/project, use list_sessions.",
	}, func(
		ctx context.Context,
		_ *sdkmcp.CallToolRequest,
		in SearchSessionsInput,
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
			resp, err := store.SearchSessions(ctx, in.Query, in.Limit)
			if err != nil {
				return nil, nil, errors.New(err.Error())
			}
			result := resultFrom(resp)
			return jsonResult(result), result, nil
		}
		if err := requireCloud(deps); err != nil {
			return nil, nil, toolErr(err)
		}

		scope := in.Scope
		if scope == "" {
			scope = "sessions"
		}
		if scope != "sessions" && scope != "pins" {
			return nil, nil, errors.New(errfmt.Format(&errfmt.UsageError{
				Message: "scope must be sessions or pins",
			}))
		}

		limit, err := searchLimit(in.Limit)
		if err != nil {
			return nil, nil, errors.New(errfmt.Format(err))
		}

		me, err := deps.Client.MeCached(ctx)
		if err != nil {
			return nil, nil, errors.New(errfmt.Format(err))
		}

		if !me.HasCapability("search") {
			if scope == "pins" {
				return nil, nil, errors.New("Pin search requires Disbug API capability \"search\"; your team's instance does not advertise it. Local fallback is currently only available for scope \"sessions\".")
			}
			// Local fallback for sessions
			resp, err := deps.Client.ListSessions(ctx, &client.ListSessionsParams{Limit: 100})
			if err != nil {
				return nil, nil, errors.New(errfmt.Format(err))
			}
			query := strings.ToLower(strings.TrimSpace(in.Query))
			var filtered []client.SessionSummary
			for _, s := range resp.Results {
				if s.Matches(query) {
					filtered = append(filtered, s)
				}
			}
			if len(filtered) > limit {
				filtered = filtered[:limit]
			}
			result := resultFrom(&client.SearchSessionsResponse{
				Results: filtered,
				Total:   len(filtered),
			})
			return jsonResult(result), result, nil
		}

		resp, err := deps.Client.SearchSessions(ctx, &client.SearchParams{
			Query: in.Query,
			Scope: scope,
			Limit: limit,
		})
		if err != nil {
			return nil, nil, errors.New(errfmt.Format(err))
		}
		if resp == nil {
			return nil, nil, errors.New("disbug API returned no search sessions")
		}

		result := resultFrom(resp)
		return jsonResult(result), result, nil
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
