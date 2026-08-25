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

const serverInstructions = `Use Disbug evidence to reproduce, diagnose, and verify bugs. Treat a cloud report URL as its stable identity and start with get_session or the selected pin's feedback. Attachment metadata includes stable IDs; call download_attachment when an attachment is relevant so the agent receives its actual content. Fetch only the smallest evidence fields needed for the current hypothesis; expand when necessary instead of requesting everything by default. Fetching is read-only: never change status merely because a report was read. After a user-requested fix has been implemented and verified, use the matching status tool to record the outcome and an optional concise note. Use inspect_local_report for downloaded Disbug JSON files. Tool schemas are the source of truth for exact inputs.`

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
	}, &mcp.ServerOptions{Instructions: serverInstructions})

	registerWhoami(srv, deps)
	registerListSessions(srv, deps)
	registerGetSession(srv, deps)
	registerGetPin(srv, deps)
	registerGetPins(srv, deps)
	registerDownloadAttachment(srv, deps)
	registerInspectLocalReport(srv, deps)
	registerSearchSessions(srv, deps)
	registerSearchPins(srv, deps)
	registerSetSessionStatus(srv, deps)
	registerSetPinStatus(srv, deps)

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
