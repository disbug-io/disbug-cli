package cmd

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWhoami(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Method, http.MethodGet; got != want {
			t.Fatalf("method = %q, want %q", got, want)
		}
		if got, want := r.URL.Path, "/api/me/"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"agent_name":"cli-agent",
			"team":"Disbug",
			"team_slug":"disbug",
			"created_by_email":"owner@example.com",
			"token_prefix":"dba_1234",
			"last_used_at":"2026-05-01T12:00:00Z",
			"api_version":"2026-05-01",
			"capabilities":["sessions:read"]
		}`)
	}))
	t.Cleanup(srv.Close)
	setupClient(t, srv)

	stdout, stderr, err := executeWhoami(t, "whoami")
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil; stderr=%q", err, stderr)
	}
	if got := stderr; got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}
	if got := stdout; !bytes.Contains([]byte(got), []byte(`"agent_name":"cli-agent"`)) {
		t.Fatalf("stdout = %q, want agent_name JSON", got)
	}
}

func executeWhoami(t *testing.T, args ...string) (string, string, error) {
	t.Helper()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := Execute(context.Background(), args, nil, &stdout, &stderr)
	return stdout.String(), stderr.String(), err
}
