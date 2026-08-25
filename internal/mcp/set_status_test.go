package mcp

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/disbug-io/disbug-cli/internal/client"
)

func TestSetPinStatus_InProcess(t *testing.T) {
	var statusBody map[string]string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/me/":
			_, _ = io.WriteString(w, `{"agent_name":"codex","capabilities":["status_updates"]}`)
		case "/api/teams/abb/projects/2/sessions/5/pins/by-number/2/status/":
			if r.Method != http.MethodPost {
				t.Fatalf("method = %q, want POST", r.Method)
			}
			if err := json.NewDecoder(r.Body).Decode(&statusBody); err != nil {
				t.Fatal(err)
			}
			_, _ = io.WriteString(w, `{"number":2,"feedback":"broken","status":"resolved","element_info":{},"metadata":{},"agent_log":[{"action":"status_changed","agent_display":"codex","pin_number":2,"status":"resolved","note":"Fixed and verified.","created_at":"2026-08-13T10:00:00Z"}]}`)
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	t.Cleanup(backend.Close)

	cli := client.New(backend.URL, "dba_test", "disbug-cli-test", nil, backend.Client(), nil)
	res, err := callTool(t, newServer(&Deps{Client: cli}), "set_pin_status", map[string]any{
		"target": "https://staging.disbug.us/abb/projects/2/sessions/5/?pin=2",
		"status": "resolved",
		"note":   " Fixed and verified. ",
	})
	if err != nil {
		t.Fatalf("CallTool(set_pin_status) error = %v", err)
	}
	if res.IsError {
		t.Fatalf("set_pin_status IsError = true: %#v", res.Content)
	}
	if statusBody["status"] != "resolved" || statusBody["note"] != "Fixed and verified." {
		t.Fatalf("status body = %#v", statusBody)
	}
	if text := firstTextContent(t, res); !strings.Contains(text, `"note":"Fixed and verified."`) {
		t.Fatalf("content = %q, want activity note", text)
	}
}

func TestSetSessionStatus_InProcess(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/me/":
			_, _ = io.WriteString(w, `{"agent_name":"codex","capabilities":["status_updates"]}`)
		case "/api/teams/abb/projects/2/sessions/5/status/":
			_, _ = io.WriteString(w, `{"team_slug":"abb","project_session_number":5,"status":"dismissed","pins":[],"agent_log":[]}`)
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	t.Cleanup(backend.Close)

	cli := client.New(backend.URL, "dba_test", "disbug-cli-test", nil, backend.Client(), nil)
	res, err := callTool(t, newServer(&Deps{Client: cli}), "set_session_status", map[string]any{
		"target": "https://staging.disbug.us/abb/projects/2/sessions/5/",
		"status": "dismissed",
	})
	if err != nil {
		t.Fatalf("CallTool(set_session_status) error = %v", err)
	}
	if res.IsError {
		t.Fatalf("set_session_status IsError = true: %#v", res.Content)
	}
	if text := firstTextContent(t, res); !strings.Contains(text, `"status":"dismissed"`) {
		t.Fatalf("content = %q, want dismissed status", text)
	}
}

func TestSetPinStatus_InvalidStatusDoesNotCallBackend(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected backend request: %s %s", r.Method, r.URL.String())
	}))
	t.Cleanup(backend.Close)

	cli := client.New(backend.URL, "dba_test", "disbug-cli-test", nil, backend.Client(), nil)
	res, err := callTool(t, newServer(&Deps{Client: cli}), "set_pin_status", map[string]any{
		"target": "https://staging.disbug.us/abb/projects/2/sessions/5/?pin=2",
		"status": "working",
	})
	if err != nil {
		t.Fatalf("CallTool(set_pin_status) error = %v, want tool error result", err)
	}
	if !res.IsError {
		t.Fatal("set_pin_status IsError = false, want true")
	}
}
