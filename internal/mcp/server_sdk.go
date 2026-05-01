package mcp

import (
	"context"

	mcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/disbug-io/disbug-cli/internal/errfmt"
)

func newServer(deps *Deps) *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "disbug",
		Version: versionStr(),
	}, nil)

	registerWhoami(srv, deps)
	registerListSessions(srv, deps)
	registerGetSession(srv, deps)
	registerGetPin(srv, deps)
	registerGetPins(srv, deps)
	registerSearchSessions(srv, deps)
	registerSearchPins(srv, deps)

	return srv
}

func serveStdio(ctx context.Context, srv *mcp.Server) error {
	return srv.Run(ctx, &mcp.StdioTransport{})
}

func jsonResult(v any) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: jsonText(v)}},
	}
}

func errResult(err error) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: errfmt.Format(err)}},
		IsError: true,
	}
}
