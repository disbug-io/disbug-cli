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
	Target  string `json:"target,omitempty" jsonschema:"Disbug report URL, or local report id for source=local"`
	URL     string `json:"url,omitempty" jsonschema:"Disbug report URL"`
	ID      string `json:"id,omitempty" jsonschema:"local report id e.g. local_..."`
	Session string `json:"session,omitempty" jsonschema:"local report id e.g. local_..."`
	Source  string `json:"source,omitempty" jsonschema:"Source: auto, cloud, or local"`
}

func registerGetSession(srv *sdkmcp.Server, deps *Deps) {
	sdkmcp.AddTool[GetSessionInput, Result](srv, &sdkmcp.Tool{
		Name:        "get_session",
		Description: "Get full details for a Disbug session. Use a report URL for cloud, or a local report id for source=local.",
	}, func(
		ctx context.Context,
		_ *sdkmcp.CallToolRequest,
		in GetSessionInput,
	) (*sdkmcp.CallToolResult, Result, error) {
		target := strings.TrimSpace(in.Target)
		if target == "" {
			target = strings.TrimSpace(in.URL)
		}
		if target == "" {
			target = strings.TrimSpace(in.ID)
		}
		if target == "" {
			target = strings.TrimSpace(in.Session)
		}
		source, err := routeSessionSource(in.Source, target, deps)
		if err != nil {
			return nil, nil, toolErr(err)
		}
		if source == sourceLocal {
			store, err := requireLocal(deps)
			if err != nil {
				return nil, nil, errors.New(err.Error())
			}
			resp, err := store.GetSession(ctx, target)
			if err != nil {
				return nil, nil, errors.New(mapLocalErr(target, err).Error())
			}
			result := Result(resp)
			return jsonResult(result), result, nil
		}
		if err := requireCloud(deps); err != nil {
			return nil, nil, toolErr(err)
		}

		sessionRef, err := ref.ParseSession(target)
		if err != nil {
			return nil, nil, errors.New(errfmt.Format(&errfmt.UsageError{Message: err.Error()}))
		}

		resp, err := deps.Client.GetSession(ctx, sessionRef)
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
