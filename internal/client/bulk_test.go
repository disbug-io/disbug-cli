package client

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"github.com/disbug-io/disbug-cli/internal/ref"
)

func TestGetPinsBulkPartialFailureAndPreservesOrder(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/teams/abb/projects/2/sessions/5/pins/by-number/1/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":101,"number":1,"feedback":"one"}`)
		case "/api/teams/abb/projects/2/sessions/5/pins/by-number/2/":
			w.Header().Set("X-Request-ID", "req-2")
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"code":"not_found","detail":"pin 2 missing","request_id":"req-body-2"}`)
		case "/api/teams/abb/projects/2/sessions/5/pins/by-number/3/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":103,"number":3,"feedback":"three"}`)
		case "/api/teams/abb/projects/2/sessions/5/pins/by-number/4/":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, `{"code":"internal_error","detail":"pin 4 failed","request_id":"req-4"}`)
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)
	t.Setenv("DISBUG_BULK_CONCURRENCY", "4")
	c := New(server.URL, "t", "test", nil, server.Client(), nil)
	sessionRef := ref.SessionRef{TeamSlug: "abb", ProjectID: 2, SessionNumber: 5}

	result := c.GetPinsBulk(context.Background(), []ref.PinFetch{
		{Pin: ref.PinRef{Session: sessionRef, Pin: 1}, Fields: []string{"console"}},
		{Pin: ref.PinRef{Session: sessionRef, Pin: 2}, Fields: []string{"network"}},
		{Pin: ref.PinRef{Session: sessionRef, Pin: 3}, Fields: []string{"all"}},
		{Pin: ref.PinRef{Session: sessionRef, Pin: 4}, Fields: []string{"events"}},
	})

	if got, want := len(result.Pins), 2; got != want {
		t.Fatalf("len(Pins) = %d, want %d", got, want)
	}
	if got, want := result.Pins[0].Number, int64(1); got != want {
		t.Fatalf("Pins[0].Number = %d, want %d", got, want)
	}
	if got, want := result.Pins[1].Number, int64(3); got != want {
		t.Fatalf("Pins[1].Number = %d, want %d", got, want)
	}
	if got, want := len(result.Errors), 2; got != want {
		t.Fatalf("len(Errors) = %d, want %d", got, want)
	}
	if got, want := result.Errors[0].Pin, "abb/projects/2/sessions/5?pin=2"; got != want {
		t.Fatalf("Errors[0].Pin = %q, want %q", got, want)
	}
	if got, want := result.Errors[0].Code, "not_found"; got != want {
		t.Fatalf("Errors[0].Code = %q, want %q", got, want)
	}
	if got, want := result.Errors[0].RequestID, "req-body-2"; got != want {
		t.Fatalf("Errors[0].RequestID = %q, want %q", got, want)
	}
	if got, want := result.Errors[1].Pin, "abb/projects/2/sessions/5?pin=4"; got != want {
		t.Fatalf("Errors[1].Pin = %q, want %q", got, want)
	}
	if result.AllFailed() {
		t.Fatal("AllFailed() = true, want false")
	}
}

func TestGetPinsBulkAllFailed(t *testing.T) {
	c := New("https://api.example.test", "t", "test", nil, doerFunc(func(req *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("dial %s failed", req.URL.Path)
	}), nil)
	t.Setenv("DISBUG_BULK_CONCURRENCY", "2")
	sessionRef := ref.SessionRef{TeamSlug: "abb", ProjectID: 2, SessionNumber: 5}

	result := c.GetPinsBulk(context.Background(), []ref.PinFetch{
		{Pin: ref.PinRef{Session: sessionRef, Pin: 1}, Fields: []string{"console"}},
		{Pin: ref.PinRef{Session: sessionRef, Pin: 2}, Fields: []string{"network"}},
	})

	if got, want := len(result.Pins), 0; got != want {
		t.Fatalf("len(Pins) = %d, want %d", got, want)
	}
	if got, want := len(result.Errors), 2; got != want {
		t.Fatalf("len(Errors) = %d, want %d", got, want)
	}
	if !result.AllFailed() {
		t.Fatal("AllFailed() = false, want true")
	}
	if got, want := result.FirstFailureExitCode(), 5; got != want {
		t.Fatalf("FirstFailureExitCode() = %d, want %d", got, want)
	}
}

func TestGetPinsBulkPreCanceledContextReportsEveryPin(t *testing.T) {
	c := New("https://api.example.test", "t", "test", nil, doerFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("HTTP doer should not be called for a pre-canceled context")
		return nil, nil
	}), nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	sessionRef := ref.SessionRef{TeamSlug: "abb", ProjectID: 2, SessionNumber: 5}

	result := c.GetPinsBulk(ctx, []ref.PinFetch{
		{Pin: ref.PinRef{Session: sessionRef, Pin: 1}, Fields: []string{"all"}},
		{Pin: ref.PinRef{Session: sessionRef, Pin: 2}, Fields: []string{"all"}},
	})

	if got, want := len(result.Pins), 0; got != want {
		t.Fatalf("len(Pins) = %d, want %d", got, want)
	}
	if got, want := len(result.Errors), 2; got != want {
		t.Fatalf("len(Errors) = %d, want %d", got, want)
	}
	if got, want := result.Errors[0].Pin, "abb/projects/2/sessions/5?pin=1"; got != want {
		t.Fatalf("Errors[0].Pin = %q, want %q", got, want)
	}
	if got, want := result.Errors[1].Pin, "abb/projects/2/sessions/5?pin=2"; got != want {
		t.Fatalf("Errors[1].Pin = %q, want %q", got, want)
	}
	for i, item := range result.Errors {
		if got, want := item.Code, "network_error"; got != want {
			t.Fatalf("Errors[%d].Code = %q, want %q", i, got, want)
		}
		if item.Message == "" {
			t.Fatalf("Errors[%d].Message is empty, want cancellation message", i)
		}
	}
	if !result.AllFailed() {
		t.Fatal("AllFailed() = false, want true")
	}
	if got, want := result.FirstFailureExitCode(), 5; got != want {
		t.Fatalf("FirstFailureExitCode() = %d, want %d", got, want)
	}
}

func TestGetPinsBulkCancellationBeforeAllDispatchReportsUndispatchedPins(t *testing.T) {
	t.Setenv("DISBUG_BULK_CONCURRENCY", "1")
	firstRequestStarted := make(chan struct{})
	var signalFirstRequestOnce sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		signalFirstRequestOnce.Do(func() {
			close(firstRequestStarted)
		})
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)

	c := New(server.URL, "t", "test", nil, server.Client(), nil)
	ctx, cancel := context.WithCancel(context.Background())
	sessionRef := ref.SessionRef{TeamSlug: "abb", ProjectID: 2, SessionNumber: 5}
	done := make(chan BulkResult, 1)
	go func() {
		done <- c.GetPinsBulk(ctx, []ref.PinFetch{
			{Pin: ref.PinRef{Session: sessionRef, Pin: 1}, Fields: []string{"all"}},
			{Pin: ref.PinRef{Session: sessionRef, Pin: 2}, Fields: []string{"all"}},
			{Pin: ref.PinRef{Session: sessionRef, Pin: 3}, Fields: []string{"all"}},
		})
	}()

	<-firstRequestStarted
	cancel()
	result := <-done

	if got, want := len(result.Pins), 0; got != want {
		t.Fatalf("len(Pins) = %d, want %d", got, want)
	}
	if got, want := len(result.Errors), 3; got != want {
		t.Fatalf("len(Errors) = %d, want %d", got, want)
	}
	for i, wantPin := range []string{
		"abb/projects/2/sessions/5?pin=1",
		"abb/projects/2/sessions/5?pin=2",
		"abb/projects/2/sessions/5?pin=3",
	} {
		if got := result.Errors[i].Pin; got != wantPin {
			t.Fatalf("Errors[%d].Pin = %q, want %q", i, got, wantPin)
		}
		if got, want := result.Errors[i].Code, "network_error"; got != want {
			t.Fatalf("Errors[%d].Code = %q, want %q", i, got, want)
		}
	}
	if !result.AllFailed() {
		t.Fatal("AllFailed() = false, want true")
	}
	if got, want := result.FirstFailureExitCode(), 5; got != want {
		t.Fatalf("FirstFailureExitCode() = %d, want %d", got, want)
	}
}

func TestBulkConcurrencyEnv(t *testing.T) {
	tests := []struct {
		name     string
		value    *string
		want     int
		wantWarn bool
	}{
		{name: "unset", value: nil, want: 8},
		{name: "zero", value: strPtr("0"), want: 1},
		{name: "negative", value: strPtr("-3"), want: 1},
		{name: "valid", value: strPtr("5"), want: 5},
		{name: "clamped", value: strPtr("100"), want: 32},
		{name: "invalid", value: strPtr("abc"), want: 8, wantWarn: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value == nil {
				t.Setenv("DISBUG_BULK_CONCURRENCY", "")
				if err := os.Unsetenv("DISBUG_BULK_CONCURRENCY"); err != nil {
					t.Fatalf("Unsetenv: %v", err)
				}
			} else {
				t.Setenv("DISBUG_BULK_CONCURRENCY", *tt.value)
			}

			stderr := captureStderr(t, func() {
				if got := bulkConcurrency(); got != tt.want {
					t.Fatalf("bulkConcurrency() = %d, want %d", got, tt.want)
				}
			})
			if tt.wantWarn && !bytes.Contains(stderr, []byte("warning:")) {
				t.Fatalf("stderr = %q, want warning", stderr)
			}
			if !tt.wantWarn && len(stderr) != 0 {
				t.Fatalf("stderr = %q, want empty", stderr)
			}
		})
	}
}

func TestGetPinsBulkConcurrencyCap(t *testing.T) {
	t.Setenv("DISBUG_BULK_CONCURRENCY", "2")
	var mu sync.Mutex
	inFlight := 0
	maxInFlight := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		inFlight++
		if inFlight > maxInFlight {
			maxInFlight = inFlight
		}
		mu.Unlock()

		defer func() {
			mu.Lock()
			inFlight--
			mu.Unlock()
		}()

		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)

	c := New(server.URL, "t", "test", nil, server.Client(), nil)
	ctx, cancel := context.WithCancel(context.Background())
	sessionRef := ref.SessionRef{TeamSlug: "abb", ProjectID: 2, SessionNumber: 5}
	items := []ref.PinFetch{
		{Pin: ref.PinRef{Session: sessionRef, Pin: 1}, Fields: []string{"all"}},
		{Pin: ref.PinRef{Session: sessionRef, Pin: 2}, Fields: []string{"all"}},
		{Pin: ref.PinRef{Session: sessionRef, Pin: 3}, Fields: []string{"all"}},
		{Pin: ref.PinRef{Session: sessionRef, Pin: 4}, Fields: []string{"all"}},
	}
	done := make(chan BulkResult, 1)
	go func() {
		done <- c.GetPinsBulk(ctx, items)
	}()

	for {
		mu.Lock()
		seen := maxInFlight
		mu.Unlock()
		if seen == 2 {
			break
		}
	}
	cancel()
	<-done

	mu.Lock()
	got := maxInFlight
	mu.Unlock()
	if got > 2 {
		t.Fatalf("max in-flight = %d, want <= 2", got)
	}
}

func strPtr(s string) *string {
	return &s
}

func captureStderr(t *testing.T, fn func()) []byte {
	t.Helper()
	original := os.Stderr
	readFile, writeFile, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe: %v", err)
	}
	os.Stderr = writeFile
	defer func() {
		os.Stderr = original
	}()

	fn()

	if err := writeFile.Close(); err != nil {
		t.Fatalf("close stderr pipe writer: %v", err)
	}
	out, err := io.ReadAll(readFile)
	if err != nil {
		t.Fatalf("read stderr pipe: %v", err)
	}
	if err := readFile.Close(); err != nil {
		t.Fatalf("close stderr pipe reader: %v", err)
	}
	return out
}
