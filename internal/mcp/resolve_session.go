package mcp

import (
	"context"
	"errors"
	"strings"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/disbug-io/disbug-cli/internal/errfmt"
	"github.com/disbug-io/disbug-cli/internal/ref"
)

// ResolveSessionInput is the input for the resolve_session MCP tool.
type ResolveSessionInput struct {
	URL     string `json:"url" jsonschema:"Disbug session URL"`
	Summary string `json:"summary" jsonschema:"What changed and how the fix was verified"`
}

func registerResolveSession(srv *sdkmcp.Server, deps *Deps) {
	sdkmcp.AddTool[ResolveSessionInput, Result](srv, &sdkmcp.Tool{
		Name:        "resolve_session",
		Description: "Mark a Disbug session resolved after the fix is verified, recording a required summary.",
	}, func(
		ctx context.Context,
		_ *sdkmcp.CallToolRequest,
		in ResolveSessionInput,
	) (*sdkmcp.CallToolResult, Result, error) {
		if err := requireCloud(deps); err != nil {
			return nil, nil, toolErr(err)
		}
		summary := strings.TrimSpace(in.Summary)
		if summary == "" {
			return nil, nil, errors.New(errfmt.Format(&errfmt.UsageError{Message: "summary must not be empty"}))
		}
		sessionRef, err := ref.ParseSession(strings.TrimSpace(in.URL))
		if err != nil {
			return nil, nil, errors.New(errfmt.Format(&errfmt.UsageError{Message: err.Error()}))
		}
		if err := deps.Client.RequireCapability(ctx, "resolve_session"); err != nil {
			return nil, nil, errors.New(errfmt.Format(err))
		}

		resp, err := deps.Client.ResolveSession(ctx, sessionRef, summary)
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
