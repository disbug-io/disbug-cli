package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/disbug-io/disbug-cli/internal/errfmt"
)

func TestPinsPartialFailureReturnsSuccessWithBulkErrors(t *testing.T) {
	var callsMu sync.Mutex
	calls := map[string]int{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callsMu.Lock()
		calls[r.URL.Path]++
		callsMu.Unlock()

		switch r.URL.Path {
		case "/api/me/":
			writePinCapabilities(w, "pin_field_selection", "pin_by_number")
		case "/api/sessions/7392/pins/by-number/2/":
			writePinJSON(w, 5827, 2)
		case "/api/sessions/7392/pins/by-number/99/":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"code":"not_found","detail":"x"}`)
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	setupClient(t, srv)

	stdout, stderr, err := executePins(t, "pins", "7392.2", "7392.99")

	if err != nil {
		t.Fatalf("Execute() error = %v, want nil; stderr=%q", err, stderr)
	}
	if got, want := calls["/api/sessions/7392/pins/by-number/2/"], 1; got != want {
		t.Fatalf("pin 2 calls = %d, want %d", got, want)
	}
	if got, want := calls["/api/sessions/7392/pins/by-number/99/"], 1; got != want {
		t.Fatalf("pin 99 calls = %d, want %d", got, want)
	}
	if !bytes.Contains([]byte(stdout), []byte(`"pin":"7392.99"`)) {
		t.Fatalf("stdout = %q, want failed pin ref", stdout)
	}
	if !bytes.Contains([]byte(stdout), []byte(`"error_code":"not_found"`)) {
		t.Fatalf("stdout = %q, want error code", stdout)
	}
	if got := stderr; got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}
}

func TestPinsAllFailedReturnsFirstFailureExitCodeAndWritesJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/me/":
			writePinCapabilities(w, "pin_field_selection", "pin_by_number")
		case "/api/sessions/7392/pins/by-number/2/", "/api/sessions/7392/pins/by-number/3/":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"code":"not_found","detail":"x"}`)
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	setupClient(t, srv)

	stdout, _, err := executePins(t, "pins", "7392.2", "7392.3")

	if err == nil {
		t.Fatal("Execute() error = nil, want all-failed error")
	}
	if got, want := ExitCode(err), 6; got != want {
		t.Fatalf("ExitCode() = %d, want %d", got, want)
	}
	if !bytes.Contains([]byte(stdout), []byte(`"pin":"7392.2"`)) {
		t.Fatalf("stdout = %q, want first bulk error", stdout)
	}
	if !bytes.Contains([]byte(stdout), []byte(`"error_code":"not_found"`)) {
		t.Fatalf("stdout = %q, want bulk error JSON", stdout)
	}
}

func TestPinsMalformedRefReturnsUsageAndDoesNotCallHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("server was called for invalid pin ref: %s %s", r.Method, r.URL.String())
	}))
	t.Cleanup(srv.Close)
	setupClient(t, srv)

	stdout, _, err := executePins(t, "pins", "7392.x")

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

func TestPinsBadDefaultFieldsReturnsUsageAndDoesNotCallHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("server was called for invalid fields: %s %s", r.Method, r.URL.String())
	}))
	t.Cleanup(srv.Close)
	setupClient(t, srv)

	stdout, _, err := executePins(t, "pins", "--fields", "bogus", "7392.2")

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

func TestPinsStyleAFields(t *testing.T) {
	queries := map[string]string{}
	var queriesMu sync.Mutex

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/me/":
			writePinCapabilities(w, "pin_field_selection", "pin_by_number")
		case "/api/sessions/7392/pins/by-number/2/", "/api/sessions/7392/pins/by-number/3/", "/api/sessions/7392/pins/by-number/4/":
			queriesMu.Lock()
			queries[r.URL.Path] = r.URL.Query().Get("fields")
			queriesMu.Unlock()
			writePinJSON(w, 5800, 2)
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	setupClient(t, srv)

	_, stderr, err := executePins(t, "pins", "7392.2:console", "7392.3:network,events", "7392.4")

	if err != nil {
		t.Fatalf("Execute() error = %v, want nil; stderr=%q", err, stderr)
	}
	if got, want := queries["/api/sessions/7392/pins/by-number/2/"], "console_logs"; got != want {
		t.Fatalf("pin 2 fields = %q, want %q", got, want)
	}
	if got, want := queries["/api/sessions/7392/pins/by-number/3/"], "network_logs,user_events"; got != want {
		t.Fatalf("pin 3 fields = %q, want %q", got, want)
	}
	if got, want := queries["/api/sessions/7392/pins/by-number/4/"], ""; got != want {
		t.Fatalf("pin 4 fields = %q, want omitted", got)
	}
}

func TestPinsDuplicateRefsAreFetchedOnceWithFieldUnion(t *testing.T) {
	var pinCalls int
	var fields string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/me/":
			writePinCapabilities(w, "pin_field_selection", "pin_by_number")
		case "/api/sessions/7392/pins/by-number/2/":
			pinCalls++
			fields = r.URL.Query().Get("fields")
			writePinJSON(w, 5827, 2)
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	setupClient(t, srv)

	_, stderr, err := executePins(t, "pins", "7392.2:console", "7392.2:network")

	if err != nil {
		t.Fatalf("Execute() error = %v, want nil; stderr=%q", err, stderr)
	}
	if got, want := pinCalls, 1; got != want {
		t.Fatalf("pin endpoint calls = %d, want %d", got, want)
	}
	if got, want := fields, "console_logs,network_logs"; got != want {
		t.Fatalf("fields query = %q, want %q", got, want)
	}
}

func TestPinsMissingCapabilityReturnsUserFacingErrorAndDoesNotCallPinEndpoint(t *testing.T) {
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

	stdout, stderr, err := executePins(t, "pins", "7392.2")

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

func TestPinsPrettyOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/me/":
			writePinCapabilities(w, "pin_field_selection", "pin_by_number")
		case "/api/sessions/7392/pins/by-number/2/":
			writePinJSON(w, 5827, 2)
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	setupClient(t, srv)

	stdout, stderr, err := executePins(t, "--pretty", "pins", "7392.2")

	if err != nil {
		t.Fatalf("Execute() error = %v, want nil; stderr=%q", err, stderr)
	}
	if got := stderr; got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}
	if got := stdout; !bytes.Contains([]byte(got), []byte("{\n  \"pins\": [\n    {\n      \"id\": 5827")) {
		t.Fatalf("stdout = %q, want indented JSON", got)
	}
}

func writePinJSON(w http.ResponseWriter, id int64, number int64) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprintf(w, `{"id":%d,"number":%d,"feedback":"broken","element_info":{},"metadata":{}}`, id, number)
}

func executePins(t *testing.T, args ...string) (string, string, error) {
	t.Helper()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := Execute(context.Background(), args, nil, &stdout, &stderr)
	return stdout.String(), stderr.String(), err
}
