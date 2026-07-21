package mcp

import (
	"context"
	"io"
	"os"
	"time"

	mcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/disbug-io/disbug-cli/internal/errfmt"
)

type serveFunc func(context.Context, *mcp.Server) error

const stdioEOFGrace = 100 * time.Millisecond

func newServer(deps *Deps) *mcp.Server {
	if deps == nil {
		deps = &Deps{}
	}
	if deps.Client != nil && !deps.CloudAvailable {
		deps.CloudAvailable = true
	}
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "disbug",
		Version: versionStr(),
	}, nil)

	registerWhoami(srv, deps)
	registerListSessions(srv, deps)
	registerGetSession(srv, deps)
	registerResolveSession(srv, deps)
	registerGetPin(srv, deps)
	registerGetPins(srv, deps)
	registerInspectLocalReport(srv, deps)
	registerSearchSessions(srv, deps)
	registerSearchPins(srv, deps)

	return srv
}

func serveStdio(ctx context.Context, srv *mcp.Server) error {
	return serveStdioWith(ctx, srv, os.Stdin, stdioWriteCloser{Writer: os.Stdout})
}

func serveStdioWith(ctx context.Context, srv *mcp.Server, stdin io.ReadCloser, stdout io.WriteCloser) error {
	return srv.Run(ctx, &mcp.IOTransport{
		Reader: delayedEOFReadCloser{ReadCloser: stdin, delay: stdioEOFGrace},
		Writer: stdout,
	})
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

type delayedEOFReadCloser struct {
	io.ReadCloser
	delay time.Duration
}

func (r delayedEOFReadCloser) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	if err == io.EOF && r.delay > 0 {
		time.Sleep(r.delay)
	}
	return n, err
}

type stdioWriteCloser struct {
	io.Writer
}

func (stdioWriteCloser) Close() error { return nil }
