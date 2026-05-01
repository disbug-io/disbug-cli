package mcp

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/disbug-io/disbug-cli/internal/client"
)

func TestGetSession_InProcess(t *testing.T) {
	requests := make(chan *http.Request, 1)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Clone(r.Context())
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"id":7392,
			"status":"open",
			"project":{"slug":"web","name":"Web"},
			"reporter":{"email":"user@example.test","display_name":"User"},
			"url":"https://example.test",
			"updated_at":"2026-05-01T00:00:00Z",
			"pins":[{"id":44,"number":2,"feedback":"button missing","url":null,"selector":null,"element_info":{},"metadata":{}}]
		}`)
	}))
	t.Cleanup(backend.Close)

	cli := client.New(backend.URL, "dba_test", "disbug-cli-test", nil, backend.Client(), nil)
	srv := newServer(&Deps{Client: cli})

	res, err := callTool(t, srv, "get_session", map[string]any{"session": "7392"})
	if err != nil {
		t.Fatalf("CallTool(get_session) error = %v, want nil", err)
	}
	if res.IsError {
		t.Fatalf("get_session IsError = true, want false: %#v", res.Content)
	}
	text := firstTextContent(t, res)
	if !strings.Contains(text, `"id":7392`) {
		t.Fatalf("get_session content = %q, want session id", text)
	}
	if !strings.Contains(text, `"pins":[`) {
		t.Fatalf("get_session content = %q, want pins", text)
	}
	if !strings.Contains(text, "button missing") {
		t.Fatalf("get_session content = %q, want feedback", text)
	}

	req := waitForRequest(t, requests, "/api/sessions/7392/")
	if got, want := req.Method, http.MethodGet; got != want {
		t.Fatalf("request method = %q, want %q", got, want)
	}
	if got, want := req.Header.Get("Authorization"), "Bearer dba_test"; got != want {
		t.Fatalf("Authorization = %q, want %q", got, want)
	}
}

func TestGetSession_InvalidInputReturnsToolError(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected backend request: %s %s", r.Method, r.URL.String())
	}))
	t.Cleanup(backend.Close)

	cli := client.New(backend.URL, "dba_test", "disbug-cli-test", nil, backend.Client(), nil)
	srv := newServer(&Deps{Client: cli})

	res, err := callTool(t, srv, "get_session", map[string]any{"session": "0"})
	if err != nil {
		t.Fatalf("CallTool(get_session) error = %v, want nil tool error result", err)
	}
	if !res.IsError {
		t.Fatalf("get_session IsError = false, want true")
	}
	if res.StructuredContent != nil {
		t.Fatalf("get_session StructuredContent = %#v, want nil on tool error", res.StructuredContent)
	}
}
