package mcp

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/disbug-io/disbug-cli/internal/client"
)

func TestGetPin_InProcess(t *testing.T) {
	requests := make(chan *http.Request, 3)
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
				"capabilities":["pin_field_selection","scoped_pin_lookup"]
			}`)
		case "/api/teams/abb/projects/2/sessions/5/pins/by-number/2/":
			_, _ = io.WriteString(w, `{
				"id":44,
				"number":2,
				"feedback":"button still broken",
				"url":null,
				"selector":null,
				"element_info":{},
				"metadata":{},
				"screenshot":null,
				"session_replay":null,
				"voice_note":null,
				"video_recording":null,
				"console":[{"message":"boom"}],
				"network":[{"url":"/api/save"}],
				"events":null
			}`)
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	t.Cleanup(backend.Close)

	cli := client.New(backend.URL, "dba_test", "disbug-cli-test", nil, backend.Client(), nil)
	srv := newServer(&Deps{Client: cli})

	res, err := callTool(t, srv, "get_pin", map[string]any{
		"pin":    "https://staging.disbug.us/abb/projects/2/sessions/5/?pin=2",
		"fields": []string{"console", "network"},
	})
	if err != nil {
		t.Fatalf("CallTool(get_pin) error = %v, want nil", err)
	}
	if res.IsError {
		t.Fatalf("get_pin IsError = true, want false: %#v", res.Content)
	}
	text := firstTextContent(t, res)
	if !strings.Contains(text, "button still broken") {
		t.Fatalf("get_pin content = %q, want pin feedback", text)
	}

	req := waitForRequest(t, requests, "/api/me/")
	if got, want := req.Header.Get("Authorization"), "Bearer dba_test"; got != want {
		t.Fatalf("/api/me/ Authorization = %q, want %q", got, want)
	}
	req = waitForRequest(t, requests, "/api/teams/abb/projects/2/sessions/5/pins/by-number/2/")
	if got, want := req.Method, http.MethodGet; got != want {
		t.Fatalf("pin request method = %q, want %q", got, want)
	}
	if got, want := req.URL.Query().Get("fields"), "console_logs,network_logs"; got != want {
		t.Fatalf("fields query = %q, want %q", got, want)
	}
	if got, want := req.Header.Get("Authorization"), "Bearer dba_test"; got != want {
		t.Fatalf("pin Authorization = %q, want %q", got, want)
	}
}

func TestGetPin_MissingCapabilityReturnsToolError(t *testing.T) {
	pinEndpointCalled := false
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/me/":
			_, _ = io.WriteString(w, `{
				"agent_name":"claude-test",
				"team":"Disbug",
				"team_slug":"disbug",
				"api_version":"2026-05-01",
				"capabilities":["scoped_pin_lookup"]
			}`)
		case "/api/teams/abb/projects/2/sessions/5/pins/by-number/2/":
			pinEndpointCalled = true
			t.Fatalf("pin endpoint called despite missing capability")
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	t.Cleanup(backend.Close)

	cli := client.New(backend.URL, "dba_test", "disbug-cli-test", nil, backend.Client(), nil)
	srv := newServer(&Deps{Client: cli})

	res, err := callTool(t, srv, "get_pin", map[string]any{"pin": "https://staging.disbug.us/abb/projects/2/sessions/5/?pin=2"})
	if err != nil {
		t.Fatalf("CallTool(get_pin) error = %v, want nil tool error result", err)
	}
	if !res.IsError {
		t.Fatalf("get_pin IsError = false, want true")
	}
	if res.StructuredContent != nil {
		t.Fatalf("get_pin StructuredContent = %#v, want nil on tool error", res.StructuredContent)
	}
	if pinEndpointCalled {
		t.Fatal("pin endpoint called despite missing capability")
	}
}

func TestGetPin_InvalidFieldsReturnsToolError(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected backend request: %s %s", r.Method, r.URL.String())
	}))
	t.Cleanup(backend.Close)

	cli := client.New(backend.URL, "dba_test", "disbug-cli-test", nil, backend.Client(), nil)
	srv := newServer(&Deps{Client: cli})

	res, err := callTool(t, srv, "get_pin", map[string]any{
		"pin":    "https://staging.disbug.us/abb/projects/2/sessions/5/?pin=2",
		"fields": []string{"console", "unknown"},
	})
	if err != nil {
		t.Fatalf("CallTool(get_pin) error = %v, want nil tool error result", err)
	}
	if !res.IsError {
		t.Fatalf("get_pin IsError = false, want true")
	}
	if res.StructuredContent != nil {
		t.Fatalf("get_pin StructuredContent = %#v, want nil on tool error", res.StructuredContent)
	}
}
