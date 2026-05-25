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
	"github.com/disbug-io/disbug-cli/internal/setup"
)

func TestDoctor_Healthy(t *testing.T) {
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
			"api_version":"2026-05-01",
			"capabilities":["search","pin_field_selection","pin_by_number"]
		}`)
	}))
	t.Cleanup(srv.Close)
	setupClient(t, srv)

	stdout, stderr, err := executeDoctor(t, "doctor")
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil; stderr=%q", err, stderr)
	}
	if got := stderr; got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}

	for _, want := range []string{
		"profile: default",
		"api_url: " + srv.URL,
		"/api/me/ OK",
		"agent_name: cli-agent",
		"team_slug: disbug",
		"api_version: 2026-05-01",
		"capabilities OK",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout = %q, want %q", stdout, want)
		}
	}
}

func TestDoctor_MissingCapability(t *testing.T) {
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
			"team_slug":"disbug",
			"api_version":"2026-05-01",
			"capabilities":["search"]
		}`)
	}))
	t.Cleanup(srv.Close)
	setupClient(t, srv)

	stdout, _, err := executeDoctor(t, "doctor")

	if err == nil {
		t.Fatal("Execute() error = nil, want missing capability error")
	}
	var userErr *errfmt.UserFacingError
	if !errors.As(err, &userErr) {
		t.Fatalf("Execute() error = %T, want errfmt.UserFacingError", err)
	}
	for _, want := range []string{
		"capabilities MISSING",
		"pin_field_selection",
		"pin_by_number",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout = %q, want %q", stdout, want)
		}
	}
}

func TestDoctor_MeServerErrorPrintsFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Method, http.MethodGet; got != want {
			t.Fatalf("method = %q, want %q", got, want)
		}
		if got, want := r.URL.Path, "/api/me/"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"code":"server_error","detail":"database password leaked","request_id":"req_123"}`)
	}))
	t.Cleanup(srv.Close)
	setupClient(t, srv)

	stdout, stderr, err := executeDoctor(t, "doctor")

	if err == nil {
		t.Fatal("Execute() error = nil, want API error")
	}
	var apiErr *errfmt.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("Execute() error = %T, want errfmt.APIError", err)
	}
	sanitized := "Server error (500). Request ID: req_123. Try again, or report this ID to support."
	if !strings.Contains(stdout, "/api/me/ FAIL - "+sanitized) {
		t.Fatalf("stdout = %q, want sanitized FAIL line", stdout)
	}
	if strings.Contains(stdout, "database password leaked") {
		t.Fatalf("stdout = %q, want no raw detail", stdout)
	}
	if !strings.Contains(stderr, sanitized) {
		t.Fatalf("stderr = %q, want sanitized error", stderr)
	}
	if strings.Contains(stderr, "database password leaked") {
		t.Fatalf("stderr = %q, want no raw detail", stderr)
	}
}

func executeDoctor(t *testing.T, args ...string) (string, string, error) {
	t.Helper()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := Execute(context.Background(), args, nil, &stdout, &stderr)
	return stdout.String(), stderr.String(), err
}

func TestSummarizeManifestDiagnostics(t *testing.T) {
	t.Run("all registered", func(t *testing.T) {
		got := summarizeManifestDiagnostics([]setup.ManifestDiagnostic{
			{Status: "registered"},
			{Status: "registered"},
		})
		if got != "registered" {
			t.Fatalf("summarizeManifestDiagnostics() = %q, want %q", got, "registered")
		}
	})

	t.Run("issues detected", func(t *testing.T) {
		got := summarizeManifestDiagnostics([]setup.ManifestDiagnostic{
			{Status: "registered"},
			{Status: "missing"},
			{Status: "outdated"},
		})
		if got != "issues detected (2/3 registrations need attention)" {
			t.Fatalf("summarizeManifestDiagnostics() = %q", got)
		}
	})
}
