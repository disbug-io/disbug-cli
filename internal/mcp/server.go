// Package mcp registers the Disbug MCP tools and runs the stdio JSON-RPC server.
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/disbug-io/disbug-cli/internal/client"
	"github.com/disbug-io/disbug-cli/internal/errfmt"
	"github.com/disbug-io/disbug-cli/internal/localstore"
	"github.com/disbug-io/disbug-cli/internal/token"
)

const defaultAPIURL = "https://disbug.io"

var versionStr = func() string { return "dev" }

// SetVersion is called by the cmd package at startup so the MCP server reports the right version.
func SetVersion(s string) { versionStr = func() string { return s } }

// Deps holds shared dependencies for MCP tool handlers.
type Deps struct {
	Client         *client.Client
	LocalStore     *localstore.Store
	Me             *client.Me
	Stderr         io.Writer
	CloudAvailable bool
}

// Result is a generic JSON tool result payload.
type Result map[string]any

// Run registers the MCP server and serves stdio JSON-RPC until stdin closes or the process is interrupted.
func Run(ctx context.Context, profile string, stderr io.Writer) error {
	return run(ctx, profile, stderr, serveStdio)
}

func run(ctx context.Context, profile string, stderr io.Writer, serveFn serveFunc) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if stderr == nil {
		stderr = io.Discard
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			writef(stderr, "mcp panic recovered: %v\n", recovered)
			os.Exit(9)
		}
	}()

	tok, err := token.Read(profile)
	if err != nil {
		writef(stderr, "warning: %s\n", errfmt.Format(err))
	}

	apiURL := tok.APIURL
	if apiURL == "" {
		apiURL = defaultAPIURL
	}
	ua := fmt.Sprintf("disbug-cli/mcp/%s (%s/%s)", versionStr(), runtime.GOOS, runtime.GOARCH)
	cli := client.New(apiURL, tok.Token, ua, nil, nil, nil)

	var me *client.Me
	if tok.Token != "" {
		me, err = cli.Me(ctx)
		if err != nil {
			writef(stderr, "warning: %s\n", errfmt.Format(err))
		}
	}

	local, localErr := localstore.Open("")
	if localErr != nil {
		writef(stderr, "warning: local store unavailable: %s\n", localErr)
	}
	if local != nil {
		defer local.Close()
	}

	deps := &Deps{Client: cli, LocalStore: local, Me: me, Stderr: stderr, CloudAvailable: tok.Token != ""}
	srv := newServer(deps)

	serveCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	serveDone := make(chan error, 1)
	go func() {
		serveDone <- serveFn(serveCtx, srv)
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	select {
	case err := <-serveDone:
		cancel()
		if err != nil && !errors.Is(err, io.EOF) {
			writef(stderr, "mcp: %s\n", err)
		}
		return nil
	case sig := <-sigCh:
		cancel()
		waitForServeDone(serveDone)
		os.Exit(signalExitCode(sig))
		return nil
	}
}

func waitForServeDone(serveDone <-chan error) {
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()

	select {
	case <-serveDone:
	case <-timer.C:
	}
}

func signalExitCode(sig os.Signal) int {
	switch sig {
	case syscall.SIGINT:
		return 130
	case syscall.SIGTERM:
		return 143
	default:
		return 1
	}
}

func writef(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}

func jsonText(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		data, _ = json.Marshal(map[string]string{"error": errfmt.Format(err)})
	}

	return string(data)
}
