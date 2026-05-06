package mcp

import (
	"context"
	"errors"
	"strings"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/disbug-io/disbug-cli/internal/errfmt"
	"github.com/disbug-io/disbug-cli/internal/ref"
)

// GetSessionInput is the input for the get_session MCP tool.
type GetSessionInput struct {
	ID      string `json:"id,omitempty" jsonschema:"session id e.g. 7392 or local_..."`
	Session string `json:"session,omitempty" jsonschema:"session id e.g. 7392 or local_..."`
	Source  string `json:"source,omitempty" jsonschema:"Source: auto, cloud, or local"`
}

func registerGetSession(srv *sdkmcp.Server, deps *Deps) {
	sdkmcp.AddTool[GetSessionInput, Result](srv, &sdkmcp.Tool{
		Name:        "get_session",
		Description: "Get full details for a Disbug session, including pins, from cloud or local source.",
	}, func(
		ctx context.Context,
		_ *sdkmcp.CallToolRequest,
		in GetSessionInput,
	) (*sdkmcp.CallToolResult, Result, error) {
		id := strings.TrimSpace(in.ID)
		if id == "" {
			id = strings.TrimSpace(in.Session)
		}
		source, err := routeSessionSource(in.Source, id, deps)
		if err != nil {
			return nil, nil, toolErr(err)
		}
		if source == sourceLocal {
			store, err := requireLocal(deps)
			if err != nil {
				return nil, nil, errors.New(err.Error())
			}
			resp, err := store.GetSession(ctx, id)
			if err != nil {
				return nil, nil, errors.New(mapLocalErr(id, err).Error())
			}
			result := Result(resp)
			return jsonResult(result), result, nil
		}
		if err := requireCloud(deps); err != nil {
			return nil, nil, toolErr(err)
		}

		sessionRef, err := ref.ParseSession(id)
		if err != nil {
			return nil, nil, errors.New(errfmt.Format(&errfmt.UsageError{Message: err.Error()}))
		}

		resp, err := deps.Client.GetSession(ctx, sessionRef.ID)
		if err != nil {
			return nil, nil, errors.New(errfmt.Format(err))
		}
		if resp == nil {
			return nil, nil, errors.New("disbug API returned no session")
		}

		result := resultFrom(resp)
		return jsonResult(result), result, nil
	})
}
