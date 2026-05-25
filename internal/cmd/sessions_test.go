package cmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/disbug-io/disbug-cli/internal/errfmt"
	"github.com/disbug-io/disbug-cli/internal/token"
)

func TestSessionsBasicList(t *testing.T) {
	var called bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if got, want := r.URL.Path, "/api/sessions/"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		// filters are handled locally
		if got := r.URL.Query().Get("status"); got != "" {
			t.Fatalf("status query = %q, want empty", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"results":[
				{
					"id":123,
					"project":{"slug":"web","name":"Website"},
					"url":"https://example.test/page",
					"status":"open",
					"pin_count":2,
					"first_pin_feedback":"broken button",
					"reporter":{"email":"r@example.test","display_name":"Reporter"},
					"updated_at":"2026-05-01T12:00:00Z",
					"free_tier_locked":false
				},
				{
					"id":124,
					"status":"resolved"
				}
			],
			"next_cursor":null,
			"count":2,
			"free_tier_truncated":false
		}`)
	}))
	t.Cleanup(srv.Close)
	setupClient(t, srv)

	stdout, stderr, err := executeSessions(t, "sessions", "--status", "open")
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil; stderr=%q", err, stderr)
	}
	if !called {
		t.Fatal("server was not called")
	}
	if got := stderr; got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}
	// Verify only one result (id:123) is in output due to local filtering
	if !bytes.Contains([]byte(stdout), []byte(`"id":123`)) {
		t.Fatalf("stdout = %q, want session id 123", stdout)
	}
	if bytes.Contains([]byte(stdout), []byte(`"id":124`)) {
		t.Fatalf("stdout = %q, should not contain id 124 (resolved)", stdout)
	}
	if got := stdout; !bytes.Contains([]byte(got), []byte(`"status":"open"`)) {
		t.Fatalf("stdout = %q, want session status", got)
	}
}

func TestSessionsIncludesQueryParams(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		// project is handled locally
		if got := query.Get("project"); got != "" {
			t.Fatalf("project query = %q, want empty", got)
		}
		// limit is overridden to 100 for local filtering
		if got, want := query.Get("limit"), "100"; got != want {
			t.Fatalf("limit query = %q, want %q", got, want)
		}
		if got, want := query.Get("cursor"), "next-1"; got != want {
			t.Fatalf("cursor query = %q, want %q", got, want)
		}
		if got := query.Get("since"); got != "" {
			t.Fatalf("since query = %q, want empty", got)
		}
		createdAtAfter := query.Get("created_at_after")
		if createdAtAfter == "" {
			t.Fatal("created_at_after query is empty")
		}
		parsed, err := time.Parse(time.RFC3339, createdAtAfter)
		if err != nil {
			t.Fatalf("created_at_after query = %q, want RFC3339 timestamp: %v", createdAtAfter, err)
		}
		if parsed.Before(time.Now().UTC().Add(-2*time.Hour-5*time.Second)) ||
			parsed.After(time.Now().UTC().Add(-2*time.Hour+5*time.Second)) {
			t.Fatalf("created_at_after query = %s, want about 2h ago", parsed)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"results":[],"next_cursor":null,"count":0,"free_tier_truncated":false}`)
	}))
	t.Cleanup(srv.Close)
	setupClient(t, srv)

	_, stderr, err := executeSessions(t, "sessions", "--project", "web", "--limit", "25", "--cursor", "next-1", "--since", "2h")
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil; stderr=%q", err, stderr)
	}
	if got := stderr; got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}
}

func TestSessionsInvalidSinceReturnsUsageAndDoesNotCallHTTP(t *testing.T) {
	var called bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	setupClient(t, srv)

	stdout, stderr, err := executeSessions(t, "sessions", "--since", "1d")

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
	if got, want := usage.Message, "--since must be a duration using s, m, or h up to 8760h"; got != want {
		t.Fatalf("usage message = %q, want %q", got, want)
	}
	if called {
		t.Fatal("server was called for invalid since")
	}
	if got := stdout; got != "" {
		t.Fatalf("stdout = %q, want empty", got)
	}
	if got := stderr; got != "--since must be a duration using s, m, or h up to 8760h\n" {
		t.Fatalf("stderr = %q, want usage message", got)
	}
}

func TestSessionsInvalidLimitReturnsUsageAndDoesNotCallHTTP(t *testing.T) {
	for _, limit := range []string{"0", "101"} {
		t.Run(limit, func(t *testing.T) {
			var called bool
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusInternalServerError)
			}))
			t.Cleanup(srv.Close)
			setupClient(t, srv)

			stdout, stderr, err := executeSessions(t, "sessions", "--limit", limit)

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
			if got, want := usage.Message, "--limit must be between 1 and 100"; got != want {
				t.Fatalf("usage message = %q, want %q", got, want)
			}
			if called {
				t.Fatal("server was called for invalid limit")
			}
			if got := stdout; got != "" {
				t.Fatalf("stdout = %q, want empty", got)
			}
			if got := stderr; got != "--limit must be between 1 and 100\n" {
				t.Fatalf("stderr = %q, want usage message", got)
			}
		})
	}
}

func TestSessionsPrettyOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"results":[{
				"id":123,
				"project":null,
				"url":"https://example.test/page",
				"status":"resolved",
				"pin_count":0,
				"first_pin_feedback":"",
				"reporter":null,
				"updated_at":"2026-05-01T12:00:00Z",
				"free_tier_locked":false
			}],
			"next_cursor":null,
			"count":1,
			"free_tier_truncated":false
		}`)
	}))
	t.Cleanup(srv.Close)
	setupClient(t, srv)

	stdout, stderr, err := executeSessions(t, "--pretty", "sessions")
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil; stderr=%q", err, stderr)
	}
	if got := stderr; got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}
	if got := stdout; !bytes.Contains([]byte(got), []byte("{\n  \"results\": [\n    {\n      \"id\": 123")) {
		t.Fatalf("stdout = %q, want indented JSON", got)
	}
}

func setupClient(t *testing.T, srv *httptest.Server) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("DISBUG_LOCAL_STORE_DIR", t.TempDir())
	t.Setenv("DISBUG_TOKEN", "")
	t.Setenv("DISBUG_API_URL", "")

	if err := token.Write("default", token.Token{Token: "test-token", APIURL: srv.URL}, false); err != nil {
		t.Fatalf("token.Write() error = %v", err)
	}
}

func executeSessions(t *testing.T, args ...string) (string, string, error) {
	t.Helper()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := Execute(context.Background(), args, nil, &stdout, &stderr)
	return stdout.String(), stderr.String(), err
}
