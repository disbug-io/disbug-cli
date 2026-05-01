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

func TestGetPins_InProcessPartialSuccess(t *testing.T) {
	requests := make(chan *http.Request, 4)
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
				"capabilities":["pin_field_selection","pin_by_number"]
			}`)
		case "/api/sessions/7392/pins/by-number/2/":
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
				"network":null,
				"events":null
			}`)
		case "/api/sessions/7392/pins/by-number/3/":
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"code":"not_found","detail":"pin not found","request_id":"req-pin-3"}`)
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	t.Cleanup(backend.Close)

	cli := client.New(backend.URL, "dba_test", "disbug-cli-test", nil, backend.Client(), nil)
	srv := newServer(&Deps{Client: cli})

	res, err := callTool(t, srv, "get_pins", map[string]any{
		"items": []map[string]any{
			{"pin": "7392.2"},
			{"pin": "7392.3", "fields": []string{"network", "events"}},
		},
		"default_fields": []string{"console"},
	})
	if err != nil {
		t.Fatalf("CallTool(get_pins) error = %v, want nil", err)
	}
	if res.IsError {
		t.Fatalf("get_pins IsError = true, want false: %#v", res.Content)
	}
	text := firstTextContent(t, res)
	for _, want := range []string{`"pins"`, `"errors"`, "button still broken", `"pin":"7392.3"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("get_pins content = %q, want %q", text, want)
		}
	}

	req := waitForRequest(t, requests, "/api/me/")
	if got, want := req.Header.Get("Authorization"), "Bearer dba_test"; got != want {
		t.Fatalf("/api/me/ Authorization = %q, want %q", got, want)
	}

	pinRequests := map[string]*http.Request{}
	for range 2 {
		req := waitForAnyRequest(t, requests)
		pinRequests[req.URL.Path] = req
	}

	req = pinRequests["/api/sessions/7392/pins/by-number/2/"]
	if req == nil {
		t.Fatal("missing request for pin 7392.2")
	}
	if got, want := req.URL.Query().Get("fields"), "console_logs"; got != want {
		t.Fatalf("pin 7392.2 fields query = %q, want %q", got, want)
	}
	if got, want := req.Header.Get("Authorization"), "Bearer dba_test"; got != want {
		t.Fatalf("pin 7392.2 Authorization = %q, want %q", got, want)
	}

	req = pinRequests["/api/sessions/7392/pins/by-number/3/"]
	if req == nil {
		t.Fatal("missing request for pin 7392.3")
	}
	if got, want := req.URL.Query().Get("fields"), "network_logs,user_events"; got != want {
		t.Fatalf("pin 7392.3 fields query = %q, want %q", got, want)
	}
	if got, want := req.Header.Get("Authorization"), "Bearer dba_test"; got != want {
		t.Fatalf("pin 7392.3 Authorization = %q, want %q", got, want)
	}
}

func TestGetPins_AllFailedReturnsToolError(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/me/":
			_, _ = io.WriteString(w, `{
				"agent_name":"claude-test",
				"team":"Disbug",
				"team_slug":"disbug",
				"api_version":"2026-05-01",
				"capabilities":["pin_field_selection","pin_by_number"]
			}`)
		case "/api/sessions/7392/pins/by-number/2/":
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"code":"not_found","detail":"pin not found"}`)
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	t.Cleanup(backend.Close)

	cli := client.New(backend.URL, "dba_test", "disbug-cli-test", nil, backend.Client(), nil)
	srv := newServer(&Deps{Client: cli})

	res, err := callTool(t, srv, "get_pins", map[string]any{
		"items": []map[string]any{{"pin": "7392.2"}},
	})
	if err != nil {
		t.Fatalf("CallTool(get_pins) error = %v, want nil tool error result", err)
	}
	if !res.IsError {
		t.Fatalf("get_pins IsError = false, want true")
	}
	if res.StructuredContent != nil {
		t.Fatalf("get_pins StructuredContent = %#v, want nil on tool error", res.StructuredContent)
	}
}

func TestGetPins_MissingCapabilityReturnsToolError(t *testing.T) {
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
				"capabilities":["pin_by_number"]
			}`)
		case "/api/sessions/7392/pins/by-number/2/":
			pinEndpointCalled = true
			t.Fatalf("pin endpoint called despite missing capability")
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	t.Cleanup(backend.Close)

	cli := client.New(backend.URL, "dba_test", "disbug-cli-test", nil, backend.Client(), nil)
	srv := newServer(&Deps{Client: cli})

	res, err := callTool(t, srv, "get_pins", map[string]any{
		"items": []map[string]any{{"pin": "7392.2"}},
	})
	if err != nil {
		t.Fatalf("CallTool(get_pins) error = %v, want nil tool error result", err)
	}
	if !res.IsError {
		t.Fatalf("get_pins IsError = false, want true")
	}
	if res.StructuredContent != nil {
		t.Fatalf("get_pins StructuredContent = %#v, want nil on tool error", res.StructuredContent)
	}
	if pinEndpointCalled {
		t.Fatal("pin endpoint called despite missing capability")
	}
}

func TestGetPins_InvalidInputReturnsToolError(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected backend request: %s %s", r.Method, r.URL.String())
	}))
	t.Cleanup(backend.Close)

	cli := client.New(backend.URL, "dba_test", "disbug-cli-test", nil, backend.Client(), nil)
	srv := newServer(&Deps{Client: cli})

	tests := []struct {
		name string
		args map[string]any
	}{
		{
			name: "empty items",
			args: map[string]any{"items": []map[string]any{}},
		},
		{
			name: "invalid field",
			args: map[string]any{
				"items": []map[string]any{{"pin": "7392.2", "fields": []string{"console", "unknown"}}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := callTool(t, srv, "get_pins", tt.args)
			if err != nil {
				t.Fatalf("CallTool(get_pins) error = %v, want nil tool error result", err)
			}
			if !res.IsError {
				t.Fatalf("get_pins IsError = false, want true")
			}
			if res.StructuredContent != nil {
				t.Fatalf("get_pins StructuredContent = %#v, want nil on tool error", res.StructuredContent)
			}
		})
	}
}

func TestJoinFields(t *testing.T) {
	got := joinFields("7392.2", []string{"console", "network"})
	if want := "7392.2:console,network"; got != want {
		t.Fatalf("joinFields() = %q, want %q", got, want)
	}
}

func waitForAnyRequest(t *testing.T, requests <-chan *http.Request) *http.Request {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), sdkTestTimeout)
	defer cancel()

	select {
	case req := <-requests:
		if req.URL.Path == "" {
			t.Fatal("request path is empty")
		}
		return req
	case <-ctx.Done():
		t.Fatalf("timed out waiting for request: %v", ctx.Err())
	}

	return nil
}
