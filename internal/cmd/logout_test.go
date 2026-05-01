package cmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/disbug-io/disbug-cli/internal/token"
)

func TestLogout_RevokeAndDelete(t *testing.T) {
	var called bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if got, want := r.Method, http.MethodPost; got != want {
			t.Fatalf("method = %q, want %q", got, want)
		}
		if got, want := r.URL.Path, "/api/agent/revoke/"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	setupClient(t, srv)

	stdout, stderr, err := executeLogout(t, "logout")
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil; stderr=%q", err, stderr)
	}
	if !called {
		t.Fatal("server was not called")
	}
	if got := stderr; got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}
	if got := stdout; got != "" {
		t.Fatalf("stdout = %q, want empty", got)
	}
	if _, err := token.Read("default"); !errors.Is(err, token.ErrProfileNotFound) {
		t.Fatalf("token.Read() error = %v, want ErrProfileNotFound", err)
	}
}

func TestLogout_LocalOnly(t *testing.T) {
	var called bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	setupClient(t, srv)

	stdout, stderr, err := executeLogout(t, "logout", "--local-only")
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil; stderr=%q", err, stderr)
	}
	if called {
		t.Fatal("server was called")
	}
	if got := stderr; got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}
	if got := stdout; got != "" {
		t.Fatalf("stdout = %q, want empty", got)
	}
	if _, err := token.Read("default"); !errors.Is(err, token.ErrProfileNotFound) {
		t.Fatalf("token.Read() error = %v, want ErrProfileNotFound", err)
	}
}

func TestLogout_ServerErrorKeepsLocalProfile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"code":"server_error","detail":"try later"}`)
	}))
	t.Cleanup(srv.Close)
	setupClient(t, srv)

	stdout, stderr, err := executeLogout(t, "logout")
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
	if got, want := ExitCode(err), 9; got != want {
		t.Fatalf("ExitCode() = %d, want %d", got, want)
	}
	if got := stdout; got != "" {
		t.Fatalf("stdout = %q, want empty", got)
	}
	if got := stderr; !bytes.Contains([]byte(got), []byte("Server error (500)")) {
		t.Fatalf("stderr = %q, want server error", got)
	}
	if _, err := token.Read("default"); err != nil {
		t.Fatalf("token.Read() error = %v, want nil", err)
	}
}

func executeLogout(t *testing.T, args ...string) (string, string, error) {
	t.Helper()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := Execute(context.Background(), args, nil, &stdout, &stderr)
	return stdout.String(), stderr.String(), err
}
