package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type recordingSleeper struct {
	mu        sync.Mutex
	durations []time.Duration
}

func (s *recordingSleeper) Sleep(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.durations = append(s.durations, d)
}

func (s *recordingSleeper) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return len(s.durations)
}

func (s *recordingSleeper) durationAt(i int) time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.durations[i]
}

type cancelingSleeper struct {
	mu     sync.Mutex
	cancel context.CancelFunc
	count  int
}

func (s *cancelingSleeper) Sleep(time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.count++
	s.cancel()
}

func (s *cancelingSleeper) sleepCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.count
}

type sequenceTransport struct {
	mu      sync.Mutex
	actions []func(*http.Request) (*http.Response, error)
	hits    int
}

func (t *sequenceTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.hits++
	if len(t.actions) == 0 {
		return response(http.StatusNoContent, nil), nil
	}

	action := t.actions[0]
	t.actions = t.actions[1:]

	return action(req)
}

func (t *sequenceTransport) hitCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()

	return t.hits
}

type closeRecorder struct {
	io.Reader
	closed *bool
}

func (r closeRecorder) Close() error {
	*r.closed = true

	return nil
}

func TestRetryTransport_DoesNotRetryHTTPStatusWhenContextCancelled(t *testing.T) {
	sleeper := &recordingSleeper{}
	ctx, cancel := context.WithCancel(context.Background())
	firstBodyClosed := false
	base := &sequenceTransport{
		actions: []func(*http.Request) (*http.Response, error){
			func(*http.Request) (*http.Response, error) {
				cancel()
				return response(http.StatusServiceUnavailable, closeRecorder{
					Reader: http.NoBody,
					closed: &firstBodyClosed,
				}), nil
			},
			func(*http.Request) (*http.Response, error) { return response(http.StatusNoContent, nil), nil },
		},
	}
	transport := newRetryTransport(base, sleeper)

	resp, err := transport.RoundTrip(newRequest(t, ctx))
	if resp != nil {
		defer resp.Body.Close()
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RoundTrip error = %v, want context canceled", err)
	}
	if resp != nil {
		t.Fatalf("response = %#v, want nil", resp)
	}
	if got, want := base.hitCount(), 1; got != want {
		t.Fatalf("hits = %d, want %d", got, want)
	}
	if got := sleeper.count(); got != 0 {
		t.Fatalf("sleeps = %d, want 0", got)
	}
	if !firstBodyClosed {
		t.Fatal("first response body closed = false, want true")
	}
}

func TestRetryTransport_DoesNotRetryUnsafeRequestBody(t *testing.T) {
	sleeper := &recordingSleeper{}
	base := &sequenceTransport{
		actions: []func(*http.Request) (*http.Response, error){
			func(*http.Request) (*http.Response, error) { return response(http.StatusServiceUnavailable, nil), nil },
			func(*http.Request) (*http.Response, error) { return response(http.StatusNoContent, nil), nil },
		},
	}
	transport := newRetryTransport(base, sleeper)
	req := newRequest(t, context.Background())
	req.Body = io.NopCloser(strings.NewReader("payload"))

	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip error = %v, want nil", err)
	}
	defer resp.Body.Close()

	if got, want := resp.StatusCode, http.StatusServiceUnavailable; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if got, want := base.hitCount(), 1; got != want {
		t.Fatalf("hits = %d, want %d", got, want)
	}
	if got := sleeper.count(); got != 0 {
		t.Fatalf("sleeps = %d, want 0", got)
	}
}

func TestRetryTransport_RetryableNilResponseBodyDoesNotPanic(t *testing.T) {
	sleeper := &recordingSleeper{}
	base := &sequenceTransport{
		actions: []func(*http.Request) (*http.Response, error){
			func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusServiceUnavailable,
					Status:     http.StatusText(http.StatusServiceUnavailable),
					Header:     make(http.Header),
				}, nil
			},
			func(*http.Request) (*http.Response, error) { return response(http.StatusNoContent, nil), nil },
		},
	}
	transport := newRetryTransport(base, sleeper)

	resp, err := transport.RoundTrip(newRequest(t, context.Background()))
	if err != nil {
		t.Fatalf("RoundTrip error = %v, want nil", err)
	}
	defer resp.Body.Close()

	if got, want := resp.StatusCode, http.StatusNoContent; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if got, want := base.hitCount(), 2; got != want {
		t.Fatalf("hits = %d, want %d", got, want)
	}
	if got, want := sleeper.count(), 1; got != want {
		t.Fatalf("sleeps = %d, want %d", got, want)
	}
}

func TestRetryTransport_Retries503ThenSucceeds(t *testing.T) {
	sleeper := &recordingSleeper{}
	base := &sequenceTransport{
		actions: []func(*http.Request) (*http.Response, error){
			func(*http.Request) (*http.Response, error) { return response(http.StatusServiceUnavailable, nil), nil },
			func(*http.Request) (*http.Response, error) { return response(http.StatusServiceUnavailable, nil), nil },
			func(*http.Request) (*http.Response, error) { return response(http.StatusNoContent, nil), nil },
		},
	}
	transport := newRetryTransport(base, sleeper)

	resp, err := transport.RoundTrip(newRequest(t, context.Background()))
	if err != nil {
		t.Fatalf("RoundTrip error = %v, want nil", err)
	}
	defer resp.Body.Close()

	if got, want := resp.StatusCode, http.StatusNoContent; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if got, want := base.hitCount(), 3; got != want {
		t.Fatalf("hits = %d, want %d", got, want)
	}
	if got, want := sleeper.count(), 2; got != want {
		t.Fatalf("sleeps = %d, want %d", got, want)
	}
	assertDurationInRange(t, sleeper.durationAt(0), 250*time.Millisecond)
	assertDurationInRange(t, sleeper.durationAt(1), 500*time.Millisecond)
}

func TestRetryTransport_DoesNotRetry400(t *testing.T) {
	sleeper := &recordingSleeper{}
	base := &sequenceTransport{
		actions: []func(*http.Request) (*http.Response, error){
			func(*http.Request) (*http.Response, error) { return response(http.StatusBadRequest, nil), nil },
		},
	}
	transport := newRetryTransport(base, sleeper)

	resp, err := transport.RoundTrip(newRequest(t, context.Background()))
	if err != nil {
		t.Fatalf("RoundTrip error = %v, want nil", err)
	}
	defer resp.Body.Close()

	if got, want := resp.StatusCode, http.StatusBadRequest; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if got, want := base.hitCount(), 1; got != want {
		t.Fatalf("hits = %d, want %d", got, want)
	}
	if got := sleeper.count(); got != 0 {
		t.Fatalf("sleeps = %d, want 0", got)
	}
}

func TestRetryTransport_Retries429WithRetryAfterZero(t *testing.T) {
	sleeper := &recordingSleeper{}
	base := &sequenceTransport{
		actions: []func(*http.Request) (*http.Response, error){
			func(*http.Request) (*http.Response, error) {
				resp := response(http.StatusTooManyRequests, nil)
				resp.Header.Set("Retry-After", "0")
				return resp, nil
			},
			func(*http.Request) (*http.Response, error) { return response(http.StatusNoContent, nil), nil },
		},
	}
	transport := newRetryTransport(base, sleeper)

	resp, err := transport.RoundTrip(newRequest(t, context.Background()))
	if err != nil {
		t.Fatalf("RoundTrip error = %v, want nil", err)
	}
	defer resp.Body.Close()

	if got, want := resp.StatusCode, http.StatusNoContent; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if got, want := base.hitCount(), 2; got != want {
		t.Fatalf("hits = %d, want %d", got, want)
	}
	if got, want := sleeper.count(), 1; got != want {
		t.Fatalf("sleeps = %d, want %d", got, want)
	}
	if got := sleeper.durationAt(0); got != 0 {
		t.Fatalf("sleep = %s, want 0", got)
	}
}

func TestRetryTransport_GivesUpAfterMaxRetries(t *testing.T) {
	sleeper := &recordingSleeper{}
	base := &sequenceTransport{
		actions: []func(*http.Request) (*http.Response, error){
			func(*http.Request) (*http.Response, error) { return response(http.StatusServiceUnavailable, nil), nil },
			func(*http.Request) (*http.Response, error) { return response(http.StatusServiceUnavailable, nil), nil },
			func(*http.Request) (*http.Response, error) { return response(http.StatusServiceUnavailable, nil), nil },
			func(*http.Request) (*http.Response, error) { return response(http.StatusServiceUnavailable, nil), nil },
		},
	}
	transport := newRetryTransport(base, sleeper)

	resp, err := transport.RoundTrip(newRequest(t, context.Background()))
	if err != nil {
		t.Fatalf("RoundTrip error = %v, want nil", err)
	}
	defer resp.Body.Close()

	if got, want := resp.StatusCode, http.StatusServiceUnavailable; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if got, want := base.hitCount(), 4; got != want {
		t.Fatalf("hits = %d, want %d", got, want)
	}
	if got, want := sleeper.count(), 3; got != want {
		t.Fatalf("sleeps = %d, want %d", got, want)
	}
}

func TestRetryTransport_RetriesNetworkErrorsThenSucceeds(t *testing.T) {
	sleeper := &recordingSleeper{}
	networkErr := errors.New("connection reset")
	base := &sequenceTransport{
		actions: []func(*http.Request) (*http.Response, error){
			func(*http.Request) (*http.Response, error) { return nil, networkErr },
			func(*http.Request) (*http.Response, error) { return nil, networkErr },
			func(*http.Request) (*http.Response, error) { return response(http.StatusNoContent, nil), nil },
		},
	}
	transport := newRetryTransport(base, sleeper)

	resp, err := transport.RoundTrip(newRequest(t, context.Background()))
	if err != nil {
		t.Fatalf("RoundTrip error = %v, want nil", err)
	}
	defer resp.Body.Close()

	if got, want := resp.StatusCode, http.StatusNoContent; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if got, want := base.hitCount(), 3; got != want {
		t.Fatalf("hits = %d, want %d", got, want)
	}
	if got, want := sleeper.count(), 2; got != want {
		t.Fatalf("sleeps = %d, want %d", got, want)
	}
}

func TestRetryTransport_DoesNotRetryWhenContextCancelled(t *testing.T) {
	sleeper := &recordingSleeper{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	base := &sequenceTransport{
		actions: []func(*http.Request) (*http.Response, error){
			func(req *http.Request) (*http.Response, error) { return nil, req.Context().Err() },
		},
	}
	transport := newRetryTransport(base, sleeper)

	resp, err := transport.RoundTrip(newRequest(t, ctx))
	if resp != nil {
		defer resp.Body.Close()
	}
	if err == nil {
		t.Fatal("RoundTrip error = nil, want context error")
	}
	if resp != nil {
		t.Fatalf("response = %#v, want nil", resp)
	}
	if got, want := base.hitCount(), 1; got != want {
		t.Fatalf("hits = %d, want %d", got, want)
	}
	if got := sleeper.count(); got != 0 {
		t.Fatalf("sleeps = %d, want 0", got)
	}
}

func TestRetryTransport_ClosesRetryableResponseBodyBeforeRetry(t *testing.T) {
	sleeper := &recordingSleeper{}
	firstBodyClosed := false
	base := &sequenceTransport{
		actions: []func(*http.Request) (*http.Response, error){
			func(*http.Request) (*http.Response, error) {
				return response(http.StatusServiceUnavailable, closeRecorder{
					Reader: http.NoBody,
					closed: &firstBodyClosed,
				}), nil
			},
			func(*http.Request) (*http.Response, error) {
				if !firstBodyClosed {
					t.Fatal("first response body was not closed before retry")
				}
				return response(http.StatusNoContent, nil), nil
			},
		},
	}
	transport := newRetryTransport(base, sleeper)

	resp, err := transport.RoundTrip(newRequest(t, context.Background()))
	if err != nil {
		t.Fatalf("RoundTrip error = %v, want nil", err)
	}
	defer resp.Body.Close()

	if !firstBodyClosed {
		t.Fatal("first response body closed = false, want true")
	}
}

func TestRetryTransport_DoesNotRetryWhenContextCancelledDuringBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	sleeper := &cancelingSleeper{cancel: cancel}
	firstBodyClosed := false
	base := &sequenceTransport{
		actions: []func(*http.Request) (*http.Response, error){
			func(*http.Request) (*http.Response, error) {
				return response(http.StatusServiceUnavailable, closeRecorder{
					Reader: http.NoBody,
					closed: &firstBodyClosed,
				}), nil
			},
			func(*http.Request) (*http.Response, error) { return response(http.StatusNoContent, nil), nil },
		},
	}
	transport := newRetryTransport(base, sleeper)

	resp, err := transport.RoundTrip(newRequest(t, ctx))
	if resp != nil {
		defer resp.Body.Close()
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RoundTrip error = %v, want context canceled", err)
	}
	if resp != nil {
		t.Fatalf("response = %#v, want nil", resp)
	}
	if got, want := base.hitCount(), 1; got != want {
		t.Fatalf("hits = %d, want %d", got, want)
	}
	if got, want := sleeper.sleepCount(), 1; got != want {
		t.Fatalf("sleeps = %d, want %d", got, want)
	}
	if !firstBodyClosed {
		t.Fatal("first response body closed = false, want true")
	}
}

func TestRetryTransport_NilBaseDefaultsToDefaultTransport(t *testing.T) {
	sleeper := &recordingSleeper{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	transport := newRetryTransport(nil, sleeper)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip error = %v, want nil", err)
	}
	defer resp.Body.Close()

	if got, want := resp.StatusCode, http.StatusNoContent; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestRetryTransport_InvalidRetryAfterFallsBackToBackoff(t *testing.T) {
	sleeper := &recordingSleeper{}
	base := &sequenceTransport{
		actions: []func(*http.Request) (*http.Response, error){
			func(*http.Request) (*http.Response, error) {
				resp := response(http.StatusTooManyRequests, nil)
				resp.Header.Set("Retry-After", "not-a-duration")
				return resp, nil
			},
			func(*http.Request) (*http.Response, error) { return response(http.StatusNoContent, nil), nil },
		},
	}
	transport := newRetryTransport(base, sleeper)

	resp, err := transport.RoundTrip(newRequest(t, context.Background()))
	if err != nil {
		t.Fatalf("RoundTrip error = %v, want nil", err)
	}
	defer resp.Body.Close()

	if got, want := sleeper.count(), 1; got != want {
		t.Fatalf("sleeps = %d, want %d", got, want)
	}
	assertDurationInRange(t, sleeper.durationAt(0), 250*time.Millisecond)
}

func newRequest(t *testing.T, ctx context.Context) *http.Request {
	t.Helper()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.disbug.test/v1/example", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	return req
}

func response(status int, body io.ReadCloser) *http.Response {
	if body == nil {
		body = http.NoBody
	}

	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     make(http.Header),
		Body:       body,
	}
}

func assertDurationInRange(t *testing.T, got time.Duration, base time.Duration) {
	t.Helper()

	min := base
	max := base + base/5
	if got < min || got > max {
		t.Fatalf("duration = %s, want between %s and %s", got, min, max)
	}
}
