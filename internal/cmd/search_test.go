package cmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/disbug-io/disbug-cli/internal/errfmt"
)

func TestSearchSessionsDefaultScope(t *testing.T) {
	var searchCalled bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/me/":
			writeSearchCapability(w, "search")
		case "/api/search/":
			searchCalled = true
			query := r.URL.Query()
			if got, want := query.Get("q"), "checkout"; got != want {
				t.Fatalf("q query = %q, want %q", got, want)
			}
			if got, want := query.Get("scope"), "sessions"; got != want {
				t.Fatalf("scope query = %q, want %q", got, want)
			}
			if got, want := query.Get("limit"), "20"; got != want {
				t.Fatalf("limit query = %q, want %q", got, want)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"results":[],"total":0}`)
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	setupClient(t, srv)

	stdout, stderr, err := executeSearch(t, "search", "checkout")
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil; stderr=%q", err, stderr)
	}
	if !searchCalled {
		t.Fatal("search endpoint was not called")
	}
	if got := stdout; !bytes.Contains([]byte(got), []byte(`"total":0`)) {
		t.Fatalf("stdout = %q, want total", got)
	}
	if got := stderr; got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}
}

func TestSearchPinsScope(t *testing.T) {
	var searchCalled bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/me/":
			writeSearchCapability(w, "search")
		case "/api/search/":
			searchCalled = true
			query := r.URL.Query()
			if got, want := query.Get("scope"), "pins"; got != want {
				t.Fatalf("scope query = %q, want %q", got, want)
			}
			if got, want := query.Get("limit"), "5"; got != want {
				t.Fatalf("limit query = %q, want %q", got, want)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{
				"results":[{
					"pin":{"id":456,"number":2,"feedback":"broken","element_info":{},"metadata":{}},
					"session":{
						"id":123,
						"project":null,
						"url":"https://example.test/page",
						"status":"open",
						"pin_count":1,
						"first_pin_feedback":"broken",
						"reporter":null,
						"updated_at":"2026-05-01T12:00:00Z",
						"free_tier_locked":false
					}
				}],
				"total":1
			}`)
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	setupClient(t, srv)

	stdout, stderr, err := executeSearch(t, "search", "checkout", "--scope", "pins", "--limit", "5")
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil; stderr=%q", err, stderr)
	}
	if !searchCalled {
		t.Fatal("search endpoint was not called")
	}
	if got := stdout; !bytes.Contains([]byte(got), []byte(`"id":456`)) {
		t.Fatalf("stdout = %q, want pin id", got)
	}
	if got := stderr; got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}
}

func TestSearchInvalidLimitReturnsUsageAndDoesNotCallHTTP(t *testing.T) {
	for _, limit := range []string{"0", "51"} {
		t.Run(limit, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Fatalf("server was called for invalid limit: %s %s", r.Method, r.URL.String())
			}))
			t.Cleanup(srv.Close)
			setupClient(t, srv)

			stdout, stderr, err := executeSearch(t, "search", "checkout", "--limit", limit)

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
			if got, want := usage.Message, "--limit must be between 1 and 50"; got != want {
				t.Fatalf("usage message = %q, want %q", got, want)
			}
			if got := stdout; got != "" {
				t.Fatalf("stdout = %q, want empty", got)
			}
			if got := stderr; got != "--limit must be between 1 and 50\n" {
				t.Fatalf("stderr = %q, want usage message", got)
			}
		})
	}
}

func TestSearchWhitespaceQueryReturnsUsageBeforeHTTP(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := SearchCmd{Query: "   ", Scope: "sessions", Limit: 20}

	err := cmd.Run(context.Background(), bindings{
		Stdout: &stdout,
		Stderr: &stderr,
		Flags:  &RootFlags{},
	})

	if err == nil {
		t.Fatal("Run() error = nil, want usage error")
	}
	if got, want := ExitCode(err), 2; got != want {
		t.Fatalf("ExitCode() = %d, want %d", got, want)
	}
	var usage *errfmt.UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("Run() error = %T, want errfmt.UsageError", err)
	}
	if got, want := usage.Message, "query is required"; got != want {
		t.Fatalf("usage message = %q, want %q", got, want)
	}
	if got := stdout.String(); got != "" {
		t.Fatalf("stdout = %q, want empty", got)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}
}

func TestSearchUnsupportedScopeReturnsUsageBeforeClient(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := SearchCmd{Query: "checkout", Scope: "everything", Limit: 20}

	err := cmd.Run(context.Background(), bindings{
		Stdout: &stdout,
		Stderr: &stderr,
		Flags:  &RootFlags{},
	})

	if err == nil {
		t.Fatal("Run() error = nil, want usage error")
	}
	if got, want := ExitCode(err), 2; got != want {
		t.Fatalf("ExitCode() = %d, want %d", got, want)
	}
	var usage *errfmt.UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("Run() error = %T, want errfmt.UsageError", err)
	}
	if got, want := usage.Message, "unsupported scope"; got != want {
		t.Fatalf("usage message = %q, want %q", got, want)
	}
	if got := stdout.String(); got != "" {
		t.Fatalf("stdout = %q, want empty", got)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}
}

func TestSearchMissingCapabilityReturnsUserFacingErrorAndDoesNotCallSearchEndpoint(t *testing.T) {
	var searchCalled bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/me/":
			writeSearchCapability(w)
		case "/api/search/":
			searchCalled = true
			w.WriteHeader(http.StatusInternalServerError)
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	setupClient(t, srv)

	stdout, stderr, err := executeSearch(t, "search", "checkout")

	if err == nil {
		t.Fatal("Execute() error = nil, want missing capability error")
	}
	var userErr *errfmt.UserFacingError
	if !errors.As(err, &userErr) {
		t.Fatalf("Execute() error = %T, want errfmt.UserFacingError", err)
	}
	if !strings.Contains(stderr, `"search"`) {
		t.Fatalf("stderr = %q, want missing capability name", stderr)
	}
	if searchCalled {
		t.Fatal("search endpoint was called despite missing capability")
	}
	if got := stdout; got != "" {
		t.Fatalf("stdout = %q, want empty", got)
	}
}

func TestSearchPrettyOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/me/":
			writeSearchCapability(w, "search")
		case "/api/search/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"results":[],"total":0}`)
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	setupClient(t, srv)

	stdout, stderr, err := executeSearch(t, "--pretty", "search", "checkout")
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil; stderr=%q", err, stderr)
	}
	if got := stdout; !bytes.Contains([]byte(got), []byte("{\n  \"results\": [],\n  \"total\": 0\n}")) {
		t.Fatalf("stdout = %q, want indented JSON", got)
	}
	if got := stderr; got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}
}

func writeSearchCapability(w http.ResponseWriter, capabilities ...string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, `{"agent_name":"agent","capabilities":[`)
	for i, capability := range capabilities {
		if i > 0 {
			_, _ = io.WriteString(w, ",")
		}
		_, _ = io.WriteString(w, `"`+capability+`"`)
	}
	_, _ = io.WriteString(w, `]}`)
}

func executeSearch(t *testing.T, args ...string) (string, string, error) {
	t.Helper()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := Execute(context.Background(), args, nil, &stdout, &stderr)
	return stdout.String(), stderr.String(), err
}
