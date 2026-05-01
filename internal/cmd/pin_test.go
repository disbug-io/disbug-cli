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

func TestPinFieldsAreWireEncoded(t *testing.T) {
	var pinCalled bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/me/":
			writePinCapabilities(w, "pin_field_selection", "pin_by_number")
		case "/api/sessions/7392/pins/by-number/2/":
			pinCalled = true
			if got, want := r.URL.Query().Get("fields"), "console_logs,network_logs"; got != want {
				t.Fatalf("fields query = %q, want %q", got, want)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":5827,"number":2,"feedback":"broken","element_info":{},"metadata":{}}`)
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	setupClient(t, srv)

	stdout, stderr, err := executePin(t, "pin", "7392.2", "--fields", "console,network")

	if err != nil {
		t.Fatalf("Execute() error = %v, want nil; stderr=%q", err, stderr)
	}
	if !pinCalled {
		t.Fatal("pin endpoint was not called")
	}
	if got := stderr; got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}
	if got := stdout; !bytes.Contains([]byte(got), []byte(`"id":5827`)) {
		t.Fatalf("stdout = %q, want pin id", got)
	}
}

func TestPinFieldsAllOmitsFieldsQuery(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/me/":
			writePinCapabilities(w, "pin_field_selection", "pin_by_number")
		case "/api/sessions/7392/pins/by-number/2/":
			if _, ok := r.URL.Query()["fields"]; ok {
				t.Fatalf("fields query present, want omitted: %q", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":5827,"number":2,"feedback":"broken","element_info":{},"metadata":{}}`)
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	setupClient(t, srv)

	_, stderr, err := executePin(t, "pin", "7392.2", "--fields", "all")

	if err != nil {
		t.Fatalf("Execute() error = %v, want nil; stderr=%q", err, stderr)
	}
	if got := stderr; got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}
}

func TestPinBadFieldReturnsUsageAndDoesNotCallHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("server was called for invalid field: %s %s", r.Method, r.URL.String())
	}))
	t.Cleanup(srv.Close)
	setupClient(t, srv)

	stdout, _, err := executePin(t, "pin", "7392.2", "--fields", "console,nope")

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
	if got := stdout; got != "" {
		t.Fatalf("stdout = %q, want empty", got)
	}
}

func TestPinBadRefReturnsUsageAndDoesNotCallHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("server was called for invalid pin ref: %s %s", r.Method, r.URL.String())
	}))
	t.Cleanup(srv.Close)
	setupClient(t, srv)

	stdout, _, err := executePin(t, "pin", "7392", "--fields", "console")

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
	if got := stdout; got != "" {
		t.Fatalf("stdout = %q, want empty", got)
	}
}

func TestPinMissingCapabilityReturnsUserFacingErrorAndDoesNotCallPinEndpoint(t *testing.T) {
	var pinCalled bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/me/":
			writePinCapabilities(w, "pin_field_selection")
		case "/api/sessions/7392/pins/by-number/2/":
			pinCalled = true
			w.WriteHeader(http.StatusInternalServerError)
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	setupClient(t, srv)

	stdout, stderr, err := executePin(t, "pin", "7392.2", "--fields", "console")

	if err == nil {
		t.Fatal("Execute() error = nil, want missing capability error")
	}
	var userErr *errfmt.UserFacingError
	if !errors.As(err, &userErr) {
		t.Fatalf("Execute() error = %T, want errfmt.UserFacingError", err)
	}
	if !strings.Contains(stderr, `"pin_by_number"`) {
		t.Fatalf("stderr = %q, want missing capability name", stderr)
	}
	if pinCalled {
		t.Fatal("pin endpoint was called despite missing capability")
	}
	if got := stdout; got != "" {
		t.Fatalf("stdout = %q, want empty", got)
	}
}

func TestPinPrettyOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/me/":
			writePinCapabilities(w, "pin_field_selection", "pin_by_number")
		case "/api/sessions/7392/pins/by-number/2/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":5827,"number":2,"feedback":"broken","element_info":{},"metadata":{}}`)
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	setupClient(t, srv)

	stdout, stderr, err := executePin(t, "--pretty", "pin", "7392.2", "--fields", "all")

	if err != nil {
		t.Fatalf("Execute() error = %v, want nil; stderr=%q", err, stderr)
	}
	if got := stderr; got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}
	if got := stdout; !bytes.Contains([]byte(got), []byte("{\n  \"id\": 5827")) {
		t.Fatalf("stdout = %q, want indented JSON", got)
	}
}

func writePinCapabilities(w http.ResponseWriter, capabilities ...string) {
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

func executePin(t *testing.T, args ...string) (string, string, error) {
	t.Helper()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := Execute(context.Background(), args, nil, &stdout, &stderr)
	return stdout.String(), stderr.String(), err
}
