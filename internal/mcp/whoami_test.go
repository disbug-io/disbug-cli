package mcp

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/disbug-io/disbug-cli/internal/client"
)

func TestWhoami_InProcess(t *testing.T) {
	requests := make(chan *http.Request, 1)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Clone(r.Context())
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"agent_name":"claude-test",
			"team":"Disbug",
			"team_slug":"disbug",
			"api_version":"2026-05-01",
			"capabilities":["sessions:read","pins:read"]
		}`)
	}))
	t.Cleanup(backend.Close)

	cli := client.New(backend.URL, "dba_test", "disbug-cli-test", nil, backend.Client(), nil)
	srv := newServer(&Deps{Client: cli})

	res, err := callTool(t, srv, "whoami", map[string]any{})
	if err != nil {
		t.Fatalf("CallTool(whoami) error = %v, want nil", err)
	}
	if res.IsError {
		t.Fatalf("whoami IsError = true, want false: %#v", res.Content)
	}
	text := firstTextContent(t, res)
	if !strings.Contains(text, "claude-test") {
		t.Fatalf("whoami content = %q, want agent name", text)
	}
	if !strings.Contains(text, `"agent_name":"claude-test"`) {
		t.Fatalf("whoami content = %q, want compact JSON field", text)
	}

	req := <-requests
	if got, want := req.Method, http.MethodGet; got != want {
		t.Fatalf("request method = %q, want %q", got, want)
	}
	if got, want := req.URL.Path, "/api/me/"; got != want {
		t.Fatalf("request path = %q, want %q", got, want)
	}
	if got, want := req.Header.Get("Authorization"), "Bearer dba_test"; got != want {
		t.Fatalf("Authorization = %q, want %q", got, want)
	}
}

func TestWhoami_InProcessAuthErrorReturnsToolError(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"code":"auth_required","detail":"token dba_secret rejected"}`)
	}))
	t.Cleanup(backend.Close)

	cli := client.New(backend.URL, "dba_test", "disbug-cli-test", nil, backend.Client(), nil)
	srv := newServer(&Deps{Client: cli})

	res, err := callTool(t, srv, "whoami", map[string]any{})
	if err != nil {
		t.Fatalf("CallTool(whoami) error = %v, want nil tool error result", err)
	}
	if !res.IsError {
		t.Fatalf("whoami IsError = false, want true")
	}
	text := firstTextContent(t, res)
	if !strings.Contains(text, "Run: disbug login") {
		t.Fatalf("whoami error content = %q, want login guidance", text)
	}
	if strings.Contains(text, "dba_secret") {
		t.Fatalf("whoami error content = %q, want sanitized auth message", text)
	}
}

func firstTextContent(t *testing.T, res *sdkmcp.CallToolResult) string {
	t.Helper()

	if res == nil {
		t.Fatal("CallToolResult = nil")
	}
	if len(res.Content) == 0 {
		t.Fatal("CallToolResult content is empty")
	}
	text, ok := res.Content[0].(*sdkmcp.TextContent)
	if !ok {
		t.Fatalf("first content type = %T, want *mcp.TextContent", res.Content[0])
	}
	if text.Text == "" {
		t.Fatal("first text content is empty")
	}

	return text.Text
}
