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

func TestResolveSession_InProcess(t *testing.T) {
	reportURL := "https://staging.disbug.us/abb/projects/2/sessions/5/"
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/me/" {
			_, _ = io.WriteString(w, `{"capabilities":["resolve_session"]}`)
			return
		}
		if got, want := r.URL.Path, "/api/teams/abb/projects/2/sessions/5/resolve/"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		if got, want := r.Method, http.MethodPost; got != want {
			t.Fatalf("method = %q, want %q", got, want)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if got, want := body["summary"], "Fixed checkout and ran the regression test."; got != want {
			t.Fatalf("summary = %q, want %q", got, want)
		}
		_, _ = io.WriteString(w, `{
			"team_slug":"abb",
			"project":{"id":2,"slug":"2","name":"Web"},
			"project_session_number":5,
			"report_url":"https://staging.disbug.us/abb/projects/2/sessions/5/",
			"status":"resolved",
			"url":"https://example.test",
			"updated_at":"2026-07-20T00:00:00Z",
			"pins":[]
		}`)
	}))
	t.Cleanup(backend.Close)

	cli := client.New(backend.URL, "dba_test", "disbug-cli-test", nil, backend.Client(), nil)
	srv := newServer(&Deps{Client: cli})
	res, err := callTool(t, srv, "resolve_session", map[string]any{
		"url":     reportURL,
		"summary": "Fixed checkout and ran the regression test.",
	})
	if err != nil {
		t.Fatalf("CallTool(resolve_session) error = %v, want nil", err)
	}
	if res.IsError {
		t.Fatalf("resolve_session IsError = true, want false: %#v", res.Content)
	}
	if text := firstTextContent(t, res); !strings.Contains(text, `"status":"resolved"`) {
		t.Fatalf("resolve_session content = %q, want resolved status", text)
	}
}

func TestResolveSession_RequiresSummary(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected backend request: %s %s", r.Method, r.URL.String())
	}))
	t.Cleanup(backend.Close)

	cli := client.New(backend.URL, "dba_test", "disbug-cli-test", nil, backend.Client(), nil)
	srv := newServer(&Deps{Client: cli})
	res, err := callTool(t, srv, "resolve_session", map[string]any{
		"url":     "https://app.disbug.io/abb/projects/2/sessions/5/",
		"summary": "  ",
	})
	if err != nil {
		t.Fatalf("CallTool(resolve_session) error = %v, want nil tool error result", err)
	}
	if !res.IsError {
		t.Fatal("resolve_session IsError = false, want true")
	}
}
