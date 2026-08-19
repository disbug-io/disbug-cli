package cmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/disbug-io/disbug-cli/internal/errfmt"
)

func TestSessionDetailSuccess(t *testing.T) {
	reportURL := "https://staging.disbug.us/abb/projects/2/sessions/5/"
	var called bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if got, want := r.URL.Path, "/api/teams/abb/projects/2/sessions/5/"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"id":7392,
			"team_slug":"abb",
			"project_session_number":5,
			"status":"open",
			"project":{"id":2,"slug":"2","name":"Website"},
			"reporter":{"email":"r@example.test","display_name":"Reporter"},
			"url":"https://example.test/page",
			"updated_at":"2026-05-01T12:00:00Z",
			"pins":[]
		}`)
	}))
	t.Cleanup(srv.Close)
	setupClient(t, srv)

	stdout, stderr, err := executeSession(t, "session", reportURL)
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil; stderr=%q", err, stderr)
	}
	if !called {
		t.Fatal("server was not called")
	}
	if got := stderr; got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}
	if got := stdout; bytes.Contains([]byte(got), []byte(`"id":7392`)) {
		t.Fatalf("stdout = %q, should not expose session database id", got)
	}
	if got := stdout; !bytes.Contains([]byte(got), []byte(`"project_session_number":5`)) {
		t.Fatalf("stdout = %q, want session number", got)
	}
}

func TestSessionBadRefReturnsUsageAndDoesNotCallHTTP(t *testing.T) {
	var called bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	setupClient(t, srv)

	stdout, _, err := executeSession(t, "session", "7392")

	if err == nil {
		t.Fatal("Execute() error = nil, want usage error")
	}
	if got, want := ExitCode(err), 2; got != want {
		t.Fatalf("ExitCode() = %d, want %d", got, want)
	}
	var usage *errfmt.UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("Execute() error = %T, want errfmt.UsageError", err)
	}
	if called {
		t.Fatal("server was called for invalid session ref")
	}
	if got := stdout; got != "" {
		t.Fatalf("stdout = %q, want empty", got)
	}
}

func TestSessionPrettyOutput(t *testing.T) {
	reportURL := "https://staging.disbug.us/abb/projects/2/sessions/5/"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"id":7392,
			"status":"resolved",
			"project":null,
			"reporter":null,
			"url":"https://example.test/page",
			"updated_at":"2026-05-01T12:00:00Z",
			"pins":[]
		}`)
	}))
	t.Cleanup(srv.Close)
	setupClient(t, srv)

	stdout, stderr, err := executeSession(t, "--pretty", "session", reportURL)
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil; stderr=%q", err, stderr)
	}
	if got := stderr; got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}
	if got := stdout; !bytes.Contains([]byte(got), []byte("{\n  \"title\":")) {
		t.Fatalf("stdout = %q, want indented JSON", got)
	}
	if got := stdout; bytes.Contains([]byte(got), []byte(`"id": 7392`)) {
		t.Fatalf("stdout = %q, should not expose session database id", got)
	}
}

func executeSession(t *testing.T, args ...string) (string, string, error) {
	t.Helper()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := Execute(context.Background(), args, nil, &stdout, &stderr)
	return stdout.String(), stderr.String(), err
}
