package mcp

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/disbug-io/disbug-cli/internal/client"
)

func TestSearchSessions_InProcessDefaultLimit(t *testing.T) {
	requests := make(chan *http.Request, 2)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Clone(r.Context())
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/me/":
			_, _ = io.WriteString(w, `{
				"agent_name":"claude-test",
				"team":"Disbug",
				"team_slug":"disbug",
				"api_version":"2026-05-01",
				"capabilities":["search"]
			}`)
		case "/api/search/":
			_, _ = io.WriteString(w, `{
				"results":[{
					"id":7392,
					"project":{"slug":"web","name":"Web"},
					"url":"https://example.test/checkout",
					"status":"open",
					"pin_count":2,
					"first_pin_feedback":"checkout button broken",
					"reporter":{"email":"user@example.test","display_name":"User"},
					"updated_at":"2026-05-01T00:00:00Z",
					"free_tier_locked":false
				}],
				"total":1
			}`)
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	t.Cleanup(backend.Close)

	cli := client.New(backend.URL, "dba_test", "disbug-cli-test", nil, backend.Client(), nil)
	srv := newServer(&Deps{Client: cli})

	res, err := callTool(t, srv, "search_sessions", map[string]any{"query": "checkout button"})
	if err != nil {
		t.Fatalf("CallTool(search_sessions) error = %v, want nil", err)
	}
	if res.IsError {
		t.Fatalf("search_sessions IsError = true, want false: %#v", res.Content)
	}
	text := firstTextContent(t, res)
	if !strings.Contains(text, `"id":7392`) {
		t.Fatalf("search_sessions content = %q, want session id", text)
	}
	if !strings.Contains(text, `"total":1`) {
		t.Fatalf("search_sessions content = %q, want total", text)
	}

	req := waitForRequest(t, requests, "/api/me/")
	if got, want := req.Header.Get("Authorization"), "Bearer dba_test"; got != want {
		t.Fatalf("/api/me/ Authorization = %q, want %q", got, want)
	}
	req = waitForRequest(t, requests, "/api/search/")
	if got, want := req.Method, http.MethodGet; got != want {
		t.Fatalf("search request method = %q, want %q", got, want)
	}
	query := req.URL.Query()
	if got, want := query.Get("q"), "checkout button"; got != want {
		t.Fatalf("q query = %q, want %q", got, want)
	}
	if got, want := query.Get("scope"), "sessions"; got != want {
		t.Fatalf("scope query = %q, want %q", got, want)
	}
	if got, want := query.Get("limit"), "20"; got != want {
		t.Fatalf("limit query = %q, want %q", got, want)
	}
}

func TestSearchSessions_CapsLimitAndAcceptsPinsScope(t *testing.T) {
	requests := make(chan *http.Request, 2)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Clone(r.Context())
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/me/":
			_, _ = io.WriteString(w, `{
				"agent_name":"claude-test",
				"team":"Disbug",
				"team_slug":"disbug",
				"api_version":"2026-05-01",
				"capabilities":["search"]
			}`)
		case "/api/search/":
			_, _ = io.WriteString(w, `{"results":[],"total":0}`)
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	t.Cleanup(backend.Close)

	cli := client.New(backend.URL, "dba_test", "disbug-cli-test", nil, backend.Client(), nil)
	srv := newServer(&Deps{Client: cli})

	res, err := callTool(t, srv, "search_sessions", map[string]any{
		"query": "mobile safari",
		"scope": "pins",
		"limit": 999,
	})
	if err != nil {
		t.Fatalf("CallTool(search_sessions) error = %v, want nil", err)
	}
	if res.IsError {
		t.Fatalf("search_sessions IsError = true, want false: %#v", res.Content)
	}

	_ = waitForRequest(t, requests, "/api/me/")
	req := waitForRequest(t, requests, "/api/search/")
	query := req.URL.Query()
	if got, want := query.Get("q"), "mobile safari"; got != want {
		t.Fatalf("q query = %q, want %q", got, want)
	}
	if got, want := query.Get("scope"), "pins"; got != want {
		t.Fatalf("scope query = %q, want %q", got, want)
	}
	if got, want := query.Get("limit"), "50"; got != want {
		t.Fatalf("limit query = %q, want %q", got, want)
	}
}

func TestSearchPins_InProcessDefaultLimit(t *testing.T) {
	requests := make(chan *http.Request, 2)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Clone(r.Context())
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/me/":
			_, _ = io.WriteString(w, `{
				"agent_name":"claude-test",
				"team":"Disbug",
				"team_slug":"disbug",
				"api_version":"2026-05-01",
				"capabilities":["search"]
			}`)
		case "/api/search/":
			_, _ = io.WriteString(w, `{
				"results":[{
					"pin":{
						"id":44,
						"number":2,
						"feedback":"checkout button broken",
						"url":null,
						"selector":"#checkout",
						"element_info":{},
						"metadata":{}
					},
					"session":{
						"id":7392,
						"project":{"slug":"web","name":"Web"},
						"url":"https://example.test/checkout",
						"status":"open",
						"pin_count":2,
						"first_pin_feedback":"checkout button broken",
						"reporter":{"email":"user@example.test","display_name":"User"},
						"updated_at":"2026-05-01T00:00:00Z",
						"free_tier_locked":false
					}
				}],
				"total":1
			}`)
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	t.Cleanup(backend.Close)

	cli := client.New(backend.URL, "dba_test", "disbug-cli-test", nil, backend.Client(), nil)
	srv := newServer(&Deps{Client: cli})

	res, err := callTool(t, srv, "search_pins", map[string]any{"query": "checkout button"})
	if err != nil {
		t.Fatalf("CallTool(search_pins) error = %v, want nil", err)
	}
	if res.IsError {
		t.Fatalf("search_pins IsError = true, want false: %#v", res.Content)
	}
	text := firstTextContent(t, res)
	if !strings.Contains(text, `"number":2`) {
		t.Fatalf("search_pins content = %q, want pin number", text)
	}
	if !strings.Contains(text, `"total":1`) {
		t.Fatalf("search_pins content = %q, want total", text)
	}

	_ = waitForRequest(t, requests, "/api/me/")
	req := waitForRequest(t, requests, "/api/search/")
	query := req.URL.Query()
	if got, want := query.Get("q"), "checkout button"; got != want {
		t.Fatalf("q query = %q, want %q", got, want)
	}
	if got, want := query.Get("scope"), "pins"; got != want {
		t.Fatalf("scope query = %q, want %q", got, want)
	}
	if got, want := query.Get("limit"), "20"; got != want {
		t.Fatalf("limit query = %q, want %q", got, want)
	}
}

func TestSearchPins_CapsLimit(t *testing.T) {
	requests := make(chan *http.Request, 2)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Clone(r.Context())
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/me/":
			_, _ = io.WriteString(w, `{
				"agent_name":"claude-test",
				"team":"Disbug",
				"team_slug":"disbug",
				"api_version":"2026-05-01",
				"capabilities":["search"]
			}`)
		case "/api/search/":
			_, _ = io.WriteString(w, `{"results":[],"total":0}`)
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	t.Cleanup(backend.Close)

	cli := client.New(backend.URL, "dba_test", "disbug-cli-test", nil, backend.Client(), nil)
	srv := newServer(&Deps{Client: cli})

	res, err := callTool(t, srv, "search_pins", map[string]any{"query": "404", "limit": 999})
	if err != nil {
		t.Fatalf("CallTool(search_pins) error = %v, want nil", err)
	}
	if res.IsError {
		t.Fatalf("search_pins IsError = true, want false: %#v", res.Content)
	}

	_ = waitForRequest(t, requests, "/api/me/")
	req := waitForRequest(t, requests, "/api/search/")
	if got, want := req.URL.Query().Get("limit"), "50"; got != want {
		t.Fatalf("limit query = %q, want %q", got, want)
	}
}

func TestSearchTools_MissingCapabilityReturnsToolError(t *testing.T) {
	tests := []struct {
		name string
		tool string
		args map[string]any
	}{
		{name: "sessions", tool: "search_sessions", args: map[string]any{"query": "checkout"}},
		{name: "pins", tool: "search_pins", args: map[string]any{"query": "checkout"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var searchEndpointCalled atomic.Bool
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/me/":
					_, _ = io.WriteString(w, `{
						"agent_name":"claude-test",
						"team":"Disbug",
						"team_slug":"disbug",
						"api_version":"2026-05-01",
						"capabilities":[]
					}`)
				case "/api/search/":
					searchEndpointCalled.Store(true)
					t.Fatalf("search endpoint called despite missing capability")
				default:
					t.Fatalf("unexpected request path: %s", r.URL.Path)
				}
			}))
			t.Cleanup(backend.Close)

			cli := client.New(backend.URL, "dba_test", "disbug-cli-test", nil, backend.Client(), nil)
			srv := newServer(&Deps{Client: cli})

			res, err := callTool(t, srv, tt.tool, tt.args)
			if err != nil {
				t.Fatalf("CallTool(%s) error = %v, want nil tool error result", tt.tool, err)
			}
			if !res.IsError {
				t.Fatalf("%s IsError = false, want true", tt.tool)
			}
			if res.StructuredContent != nil {
				t.Fatalf("%s StructuredContent = %#v, want nil on tool error", tt.tool, res.StructuredContent)
			}
			if searchEndpointCalled.Load() {
				t.Fatal("search endpoint called despite missing capability")
			}
		})
	}
}

func TestSearchTools_InvalidInputReturnsToolError(t *testing.T) {
	tests := []struct {
		name string
		tool string
		args map[string]any
		want string
	}{
		{name: "sessions empty query", tool: "search_sessions", args: map[string]any{"query": ""}, want: "query is required"},
		{name: "sessions blank query", tool: "search_sessions", args: map[string]any{"query": "   "}, want: "query is required"},
		{name: "sessions invalid scope", tool: "search_sessions", args: map[string]any{"query": "checkout", "scope": "all"}, want: "scope must be sessions or pins"},
		{name: "sessions negative limit", tool: "search_sessions", args: map[string]any{"query": "checkout", "limit": -1}, want: "limit must be greater than or equal to 0"},
		{name: "pins blank query", tool: "search_pins", args: map[string]any{"query": "   "}, want: "query is required"},
		{name: "pins negative limit", tool: "search_pins", args: map[string]any{"query": "checkout", "limit": -1}, want: "limit must be greater than or equal to 0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Fatalf("unexpected backend request: %s %s", r.Method, r.URL.String())
			}))
			t.Cleanup(backend.Close)

			cli := client.New(backend.URL, "dba_test", "disbug-cli-test", nil, backend.Client(), nil)
			srv := newServer(&Deps{Client: cli})

			res, err := callTool(t, srv, tt.tool, tt.args)
			if err != nil {
				t.Fatalf("CallTool(%s) error = %v, want nil tool error result", tt.tool, err)
			}
			if !res.IsError {
				t.Fatalf("%s IsError = false, want true", tt.tool)
			}
			text := firstTextContent(t, res)
			if !strings.Contains(text, tt.want) {
				t.Fatalf("%s error content = %q, want %q", tt.tool, text, tt.want)
			}
			if res.StructuredContent != nil {
				t.Fatalf("%s StructuredContent = %#v, want nil on tool error", tt.tool, res.StructuredContent)
			}
		})
	}
}

func TestSearchSessions_AuthErrorIsSanitized(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/me/":
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"code":"auth_required","detail":"token dba_secret rejected"}`)
		case "/api/search/":
			t.Fatalf("search endpoint called despite auth error")
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	t.Cleanup(backend.Close)

	cli := client.New(backend.URL, "dba_test", "disbug-cli-test", nil, backend.Client(), nil)
	srv := newServer(&Deps{Client: cli})

	res, err := callTool(t, srv, "search_sessions", map[string]any{"query": "checkout"})
	if err != nil {
		t.Fatalf("CallTool(search_sessions) error = %v, want nil tool error result", err)
	}
	if !res.IsError {
		t.Fatalf("search_sessions IsError = false, want true")
	}
	text := firstTextContent(t, res)
	if !strings.Contains(text, "Run: disbug login") {
		t.Fatalf("search_sessions error content = %q, want login guidance", text)
	}
	if strings.Contains(text, "dba_secret") {
		t.Fatalf("search_sessions error content = %q, want sanitized auth message", text)
	}
}

func TestSearchPins_ServerErrorIsSanitized(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/me/":
			_, _ = io.WriteString(w, `{
				"agent_name":"claude-test",
				"team":"Disbug",
				"team_slug":"disbug",
				"api_version":"2026-05-01",
				"capabilities":["search"]
			}`)
		case "/api/search/":
			w.Header().Set("X-Request-ID", "req-123")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, `{"code":"server_error","detail":"database failed for dba_secret"}`)
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	t.Cleanup(backend.Close)

	cli := client.New(backend.URL, "dba_test", "disbug-cli-test", nil, backend.Client(), nil)
	srv := newServer(&Deps{Client: cli})

	res, err := callTool(t, srv, "search_pins", map[string]any{"query": "checkout"})
	if err != nil {
		t.Fatalf("CallTool(search_pins) error = %v, want nil tool error result", err)
	}
	if !res.IsError {
		t.Fatalf("search_pins IsError = false, want true")
	}
	text := firstTextContent(t, res)
	if !strings.Contains(text, "Server error (500)") {
		t.Fatalf("search_pins error content = %q, want server error", text)
	}
	if !strings.Contains(text, "req-123") {
		t.Fatalf("search_pins error content = %q, want request id", text)
	}
	if strings.Contains(text, "dba_secret") {
		t.Fatalf("search_pins error content = %q, want sanitized server message", text)
	}
}
