package mcp

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/disbug-io/disbug-cli/internal/client"
)

func TestListSessions_InProcess(t *testing.T) {
	requests := make(chan *http.Request, 1)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Clone(r.Context())
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"results":[{
				"id":7392,
				"project":{"slug":"web","name":"Web"},
				"url":"https://example.test",
				"status":"open",
				"pin_count":2,
				"first_pin_feedback":"broken button",
				"reporter":{"email":"user@example.test","display_name":"User"},
				"updated_at":"2026-05-01T00:00:00Z",
				"free_tier_locked":false
			}],
			"next_cursor":null,
			"count":1,
			"free_tier_truncated":false
		}`)
	}))
	t.Cleanup(backend.Close)

	cli := client.New(backend.URL, "dba_test", "disbug-cli-test", nil, backend.Client(), nil)
	srv := newServer(&Deps{Client: cli})

	res, err := callTool(t, srv, "list_sessions", map[string]any{
		"limit":   25,
		"status":  "open",
		"project": "web",
	})
	if err != nil {
		t.Fatalf("CallTool(list_sessions) error = %v, want nil", err)
	}
	if res.IsError {
		t.Fatalf("list_sessions IsError = true, want false: %#v", res.Content)
	}
	text := firstTextContent(t, res)
	if !strings.Contains(text, `"id":7392`) {
		t.Fatalf("list_sessions content = %q, want session id", text)
	}
	if !strings.Contains(text, `"status":"open"`) {
		t.Fatalf("list_sessions content = %q, want compact JSON status", text)
	}

	req := waitForRequest(t, requests, "/api/sessions/")
	if got, want := req.Method, http.MethodGet; got != want {
		t.Fatalf("request method = %q, want %q", got, want)
	}
	if got, want := req.URL.Query().Get("limit"), "25"; got != want {
		t.Fatalf("limit query = %q, want %q", got, want)
	}
	if got, want := req.URL.Query().Get("status"), "open"; got != want {
		t.Fatalf("status query = %q, want %q", got, want)
	}
	if got, want := req.URL.Query().Get("project"), "web"; got != want {
		t.Fatalf("project query = %q, want %q", got, want)
	}
	if got, want := req.Header.Get("Authorization"), "Bearer dba_test"; got != want {
		t.Fatalf("Authorization = %q, want %q", got, want)
	}
}

func TestListSessions_LimitClamp(t *testing.T) {
	requests := make(chan *http.Request, 1)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Clone(r.Context())
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"results":[],"next_cursor":null,"count":0,"free_tier_truncated":false}`)
	}))
	t.Cleanup(backend.Close)

	cli := client.New(backend.URL, "dba_test", "disbug-cli-test", nil, backend.Client(), nil)
	srv := newServer(&Deps{Client: cli})

	res, err := callTool(t, srv, "list_sessions", map[string]any{"limit": 999})
	if err != nil {
		t.Fatalf("CallTool(list_sessions) error = %v, want nil", err)
	}
	if res.IsError {
		t.Fatalf("list_sessions IsError = true, want false: %#v", res.Content)
	}

	req := waitForRequest(t, requests, "/api/sessions/")
	if got, want := req.URL.Query().Get("limit"), "100"; got != want {
		t.Fatalf("limit query = %q, want %q", got, want)
	}
}

func waitForRequest(t *testing.T, requests <-chan *http.Request, path string) *http.Request {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), sdkTestTimeout)
	defer cancel()

	select {
	case req := <-requests:
		if got := req.URL.Path; got != path {
			t.Fatalf("request path = %q, want %q", got, path)
		}
		return req
	case <-ctx.Done():
		t.Fatalf("timed out waiting for %s request: %v", path, ctx.Err())
	}

	return nil
}
