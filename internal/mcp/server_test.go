package mcp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestJSONTextCompactsJSON(t *testing.T) {
	t.Parallel()

	got := jsonText(map[string]any{
		"ok":     true,
		"nested": map[string]any{"value": "hello"},
	})

	if strings.Contains(got, "\n") || strings.Contains(got, "  ") {
		t.Fatalf("jsonText returned non-compact JSON: %q", got)
	}
	if !strings.Contains(got, `"ok":true`) {
		t.Fatalf("jsonText missing compact field: %q", got)
	}
}

func TestJSONTextReturnsCompactErrorJSONOnMarshalFailure(t *testing.T) {
	t.Parallel()

	got := jsonText(map[string]any{"bad": make(chan int)})

	if strings.Contains(got, "\n") || strings.Contains(got, "  ") {
		t.Fatalf("jsonText returned non-compact error JSON: %q", got)
	}
	if !strings.Contains(got, `"error"`) {
		t.Fatalf("jsonText missing error field: %q", got)
	}
}

func TestNewServerReturnsServer(t *testing.T) {
	t.Parallel()

	if srv := newServer(nil); srv == nil {
		t.Fatal("newServer(nil) returned nil")
	}
}

func TestResultHelpersReturnTextContent(t *testing.T) {
	t.Parallel()

	ok := jsonResult(map[string]any{"ok": true})
	if len(ok.Content) != 1 {
		t.Fatalf("jsonResult content length = %d, want 1", len(ok.Content))
	}
	text, okContent := ok.Content[0].(*mcp.TextContent)
	if !okContent {
		t.Fatalf("jsonResult content type = %T, want *mcp.TextContent", ok.Content[0])
	}
	if text.Text != `{"ok":true}` {
		t.Fatalf("jsonResult text = %q, want compact JSON", text.Text)
	}

	bad := errResult(errors.New("boom"))
	if !bad.IsError {
		t.Fatal("errResult IsError = false, want true")
	}
	if len(bad.Content) != 1 {
		t.Fatalf("errResult content length = %d, want 1", len(bad.Content))
	}
	errText, okContent := bad.Content[0].(*mcp.TextContent)
	if !okContent {
		t.Fatalf("errResult content type = %T, want *mcp.TextContent", bad.Content[0])
	}
	if errText.Text != "boom" {
		t.Fatalf("errResult text = %q, want formatted error", errText.Text)
	}
}

func TestRunLogsServeStdioErrorAndReturnsNil(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	originalServeStdioFn := serveStdioFn
	t.Cleanup(func() {
		serveStdioFn = originalServeStdioFn
	})
	serveStdioFn = func(context.Context, *mcp.Server) error {
		return errors.New("loop failed")
	}

	var stderr bytes.Buffer
	err := Run(context.Background(), "default", &stderr)
	if err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	if !strings.Contains(stderr.String(), "mcp: loop failed\n") {
		t.Fatalf("stderr = %q, want mcp loop warning", stderr.String())
	}
}

func TestRunTreatsServeStdioEOFAsNormal(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	originalServeStdioFn := serveStdioFn
	t.Cleanup(func() {
		serveStdioFn = originalServeStdioFn
	})
	serveStdioFn = func(context.Context, *mcp.Server) error {
		return io.EOF
	}

	var stderr bytes.Buffer
	err := Run(context.Background(), "default", &stderr)
	if err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	if strings.Contains(stderr.String(), "mcp:") {
		t.Fatalf("stderr = %q, want no mcp loop warning", stderr.String())
	}
}
