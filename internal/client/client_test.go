package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/disbug-io/disbug-cli/internal/errfmt"
	"github.com/disbug-io/disbug-cli/internal/seams"
)

type doerFunc func(*http.Request) (*http.Response, error)

func (f doerFunc) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

type chunkedReadCloser struct {
	chunks []string
	reads  int
	eof    bool
	closed bool
}

func (b *chunkedReadCloser) Read(p []byte) (int, error) {
	b.reads++
	if len(b.chunks) == 0 {
		b.eof = true
		return 0, io.EOF
	}

	chunk := b.chunks[0]
	b.chunks = b.chunks[1:]

	return copy(p, chunk), nil
}

func (b *chunkedReadCloser) Close() error {
	b.closed = true
	return nil
}

func TestClient_DecodeAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-ID", "fallback-request")
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"code":"missing_agent","detail":"agent was not found","request_id":"envelope-request"}`)
	}))
	t.Cleanup(server.Close)

	c := New(server.URL, "dba_test", "disbug-cli-test", nil, server.Client(), nil)

	_, err := c.Me(context.Background())
	if err == nil {
		t.Fatal("Me() error = nil, want API error")
	}
	var apiErr *errfmt.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("Me() error = %T, want *errfmt.APIError", err)
	}
	if got, want := apiErr.StatusCode, http.StatusNotFound; got != want {
		t.Fatalf("StatusCode = %d, want %d", got, want)
	}
	if got, want := apiErr.Code, "missing_agent"; got != want {
		t.Fatalf("Code = %q, want %q", got, want)
	}
	if got, want := apiErr.RequestID, "envelope-request"; got != want {
		t.Fatalf("RequestID = %q, want %q", got, want)
	}
}

func TestClient_NetworkError(t *testing.T) {
	wantErr := errors.New("dial failed")
	c := New("https://api.example.test", "dba_test", "disbug-cli-test", &recordingSleeper{}, doerFunc(func(*http.Request) (*http.Response, error) {
		return nil, wantErr
	}), nil)

	_, err := c.Me(context.Background())
	if err == nil {
		t.Fatal("Me() error = nil, want network error")
	}
	var networkErr *errfmt.NetworkError
	if !errors.As(err, &networkErr) {
		t.Fatalf("Me() error = %T, want *errfmt.NetworkError", err)
	}
	if got, want := networkErr.URL, "https://api.example.test/api/me/"; got != want {
		t.Fatalf("NetworkError.URL = %q, want %q", got, want)
	}
	if !errors.Is(networkErr.Cause, wantErr) {
		t.Fatalf("NetworkError.Cause = %v, want %v", networkErr.Cause, wantErr)
	}
}

func TestClient_PreservesProvidedHTTPClientDoSemantics(t *testing.T) {
	redirectErr := errors.New("provided check redirect was used")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/api/me/final/", http.StatusFound)
	}))
	t.Cleanup(server.Close)
	provided := server.Client()
	provided.CheckRedirect = func(*http.Request, []*http.Request) error {
		return redirectErr
	}
	c := New(server.URL, "dba_test", "disbug-cli-test", &recordingSleeper{}, provided, nil)

	_, err := c.Me(context.Background())
	if err == nil {
		t.Fatal("Me() error = nil, want provided CheckRedirect error")
	}
	if !strings.Contains(err.Error(), redirectErr.Error()) {
		t.Fatalf("Me() error = %v, want provided CheckRedirect error", err)
	}
}

func TestClient_SetsDefaultTopLevelTimeout(t *testing.T) {
	c := New("https://api.example.test", "dba_test", "disbug-cli-test", nil, doerFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("doer should not be called")
		return nil, nil
	}), nil)

	if got, want := c.client.Timeout, 30*time.Second; got != want {
		t.Fatalf("client timeout = %s, want %s", got, want)
	}
}

func TestClient_MeSendsHeadersAndDecodesResponse(t *testing.T) {
	requests := make(chan *http.Request, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Clone(r.Context())
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"agent_name":"cli-agent",
			"team":"Disbug",
			"team_slug":"disbug",
			"created_by_email":"owner@example.com",
			"token_prefix":"dba_1234",
			"last_used_at":"2026-05-01T12:00:00Z",
			"api_version":"2026-05-01",
			"capabilities":["sessions:read","pins:read"]
		}`)
	}))
	t.Cleanup(server.Close)

	c := New(server.URL+"/", "dba_test", "disbug-cli-test", nil, server.Client(), nil)

	me, err := c.Me(context.Background())
	if err != nil {
		t.Fatalf("Me() error = %v, want nil", err)
	}
	if got, want := me.AgentName, "cli-agent"; got != want {
		t.Fatalf("AgentName = %q, want %q", got, want)
	}
	if got, want := me.TeamSlug, "disbug"; got != want {
		t.Fatalf("TeamSlug = %q, want %q", got, want)
	}
	if !me.HasCapability("pins:read") {
		t.Fatal("HasCapability(\"pins:read\") = false, want true")
	}

	req := <-requests
	if got, want := req.URL.Path, "/api/me/"; got != want {
		t.Fatalf("request path = %q, want %q", got, want)
	}
	if got, want := req.Header.Get("Authorization"), "Bearer dba_test"; got != want {
		t.Fatalf("Authorization = %q, want %q", got, want)
	}
	if got, want := req.Header.Get("Accept"), "application/json"; got != want {
		t.Fatalf("Accept = %q, want %q", got, want)
	}
	if got, want := req.Header.Get("User-Agent"), "disbug-cli-test"; got != want {
		t.Fatalf("User-Agent = %q, want %q", got, want)
	}
}

func TestClient_DoJSONSetsContentTypeWithBody(t *testing.T) {
	var gotContentType string
	c := New("https://api.example.test", "dba_test", "disbug-cli-test", nil, doerFunc(func(req *http.Request) (*http.Response, error) {
		gotContentType = req.Header.Get("Content-Type")
		return &http.Response{
			StatusCode: http.StatusNoContent,
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    req,
		}, nil
	}), nil)

	err := c.doJSON(context.Background(), http.MethodPost, "/api/me/", strings.NewReader(`{"ok":true}`), nil)
	if err != nil {
		t.Fatalf("doJSON() error = %v, want nil", err)
	}
	if got, want := gotContentType, "application/json"; got != want {
		t.Fatalf("Content-Type = %q, want %q", got, want)
	}
}

func TestClient_DoJSONDrainsAndClosesBodyAfterSuccessfulDecode(t *testing.T) {
	body := &chunkedReadCloser{chunks: []string{
		`{"agent_name":"agent"}`,
		` trailing bytes`,
	}}
	c := New("https://api.example.test", "dba_test", "disbug-cli-test", nil, doerFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       body,
			Request:    req,
		}, nil
	}), nil)

	var me Me
	err := c.doJSON(context.Background(), http.MethodGet, "/api/me/", nil, &me)
	if err != nil {
		t.Fatalf("doJSON() error = %v, want nil", err)
	}
	if got, want := me.AgentName, "agent"; got != want {
		t.Fatalf("AgentName = %q, want %q", got, want)
	}
	if !body.eof {
		t.Fatal("body was not drained to EOF")
	}
	if !body.closed {
		t.Fatal("body was not closed")
	}
}

func TestClient_DoJSONPreservesSuccessfulDecodeError(t *testing.T) {
	body := &chunkedReadCloser{chunks: []string{`{"agent_name":`}}
	c := New("https://api.example.test", "dba_test", "disbug-cli-test", nil, doerFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       body,
			Request:    req,
		}, nil
	}), nil)

	var me Me
	err := c.doJSON(context.Background(), http.MethodGet, "/api/me/", nil, &me)
	if err == nil {
		t.Fatal("doJSON() error = nil, want decode error")
	}
	if !body.closed {
		t.Fatal("body was not closed after decode error")
	}
}

func TestClient_DoJSONReturnsContextErrorWhileWaitingForSemaphore(t *testing.T) {
	c := New("https://api.example.test", "dba_test", "disbug-cli-test", nil, doerFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("doer should not be called while semaphore is full")
		return nil, nil
	}), nil)
	for range maxConcurrentDoJSON {
		c.sem <- struct{}{}
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := c.doJSON(ctx, http.MethodGet, "/api/me/", nil, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("doJSON() error = %v, want context.Canceled", err)
	}
	if got, want := len(c.sem), maxConcurrentDoJSON; got != want {
		t.Fatalf("semaphore length = %d, want %d", got, want)
	}
}

func TestClient_DoJSONPreservesContextTransportErrors(t *testing.T) {
	for _, errFromDoer := range []error{context.Canceled, context.DeadlineExceeded} {
		t.Run(errFromDoer.Error(), func(t *testing.T) {
			c := New("https://api.example.test", "dba_test", "disbug-cli-test", nil, doerFunc(func(*http.Request) (*http.Response, error) {
				return nil, errFromDoer
			}), nil)

			err := c.doJSON(context.Background(), http.MethodGet, "/api/me/", nil, nil)
			if !errors.Is(err, errFromDoer) {
				t.Fatalf("doJSON() error = %v, want %v", err, errFromDoer)
			}
			var networkErr *errfmt.NetworkError
			if errors.As(err, &networkErr) {
				t.Fatalf("doJSON() error = %T, want context error unchanged", err)
			}
		})
	}
}

func TestClient_UsesConcurrencySemaphoreCapacity(t *testing.T) {
	c := New("https://api.example.test", "dba_test", "disbug-cli-test", nil, doerFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("doer should not be called")
		return nil, nil
	}), nil)

	if got, want := cap(c.sem), maxConcurrentDoJSON; got != want {
		t.Fatalf("semaphore cap = %d, want %d", got, want)
	}
}

func TestClient_MeCachedCachesWithinWindowAndRefetchesAfterExpiry(t *testing.T) {
	var mu sync.Mutex
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits++
		hit := hits
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"agent_name":"agent-%d","capabilities":["sessions:read"]}`, hit)
	}))
	t.Cleanup(server.Close)
	clock := seams.NewFixedClock(time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC))
	c := New(server.URL, "dba_test", "disbug-cli-test", nil, server.Client(), clock)

	first, err := c.MeCached(context.Background())
	if err != nil {
		t.Fatalf("first MeCached() error = %v, want nil", err)
	}
	second, err := c.MeCached(context.Background())
	if err != nil {
		t.Fatalf("second MeCached() error = %v, want nil", err)
	}
	if first != second {
		t.Fatal("second MeCached() returned a different pointer within cache window")
	}
	if got, want := hitCount(&mu, &hits), 1; got != want {
		t.Fatalf("backend hits before expiry = %d, want %d", got, want)
	}

	clock.Advance(31 * time.Second)
	third, err := c.MeCached(context.Background())
	if err != nil {
		t.Fatalf("third MeCached() error = %v, want nil", err)
	}
	if got, want := third.AgentName, "agent-2"; got != want {
		t.Fatalf("third AgentName = %q, want %q", got, want)
	}
	if got, want := hitCount(&mu, &hits), 2; got != want {
		t.Fatalf("backend hits after expiry = %d, want %d", got, want)
	}
}

func TestClient_MeCachedDoesNotCacheFailures(t *testing.T) {
	var mu sync.Mutex
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits++
		hit := hits
		mu.Unlock()

		if hit <= 4 {
			http.Error(w, "temporary failure", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"agent_name":"agent","capabilities":[]}`)
	}))
	t.Cleanup(server.Close)
	c := New(server.URL, "dba_test", "disbug-cli-test", &recordingSleeper{}, server.Client(), seams.NewFixedClock(time.Now()))

	if _, err := c.MeCached(context.Background()); err == nil {
		t.Fatal("first MeCached() error = nil, want error")
	}
	me, err := c.MeCached(context.Background())
	if err != nil {
		t.Fatalf("second MeCached() error = %v, want nil", err)
	}
	if got, want := me.AgentName, "agent"; got != want {
		t.Fatalf("AgentName = %q, want %q", got, want)
	}
	if got, want := hitCount(&mu, &hits), 5; got != want {
		t.Fatalf("backend hits = %d, want %d", got, want)
	}
}

func TestClient_RequireCapability(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"agent_name":"agent","capabilities":["sessions:read"]}`)
	}))
	t.Cleanup(server.Close)
	c := New(server.URL, "dba_test", "disbug-cli-test", nil, server.Client(), seams.NewFixedClock(time.Now()))

	if err := c.RequireCapability(context.Background(), "sessions:read"); err != nil {
		t.Fatalf("RequireCapability(existing) error = %v, want nil", err)
	}
	err := c.RequireCapability(context.Background(), "pins:read")
	if err == nil {
		t.Fatal("RequireCapability(missing) error = nil, want user-facing error")
	}
	var userErr *errfmt.UserFacingError
	if !errors.As(err, &userErr) {
		t.Fatalf("RequireCapability(missing) error = %T, want *errfmt.UserFacingError", err)
	}
	if !strings.Contains(userErr.Message, `"pins:read"`) {
		t.Fatalf("UserFacingError.Message = %q, want capability name", userErr.Message)
	}
}

func TestClient_DoJSONFallbackAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-ID", "header-request")
		http.Error(w, "plain text", http.StatusTooManyRequests)
	}))
	t.Cleanup(server.Close)
	c := New(server.URL, "dba_test", "disbug-cli-test", nil, server.Client(), nil)

	err := c.doJSON(context.Background(), http.MethodGet, "api/me/", nil, nil)
	if err == nil {
		t.Fatal("doJSON() error = nil, want API error")
	}
	var apiErr *errfmt.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("doJSON() error = %T, want *errfmt.APIError", err)
	}
	if got, want := apiErr.Code, "rate_limited"; got != want {
		t.Fatalf("Code = %q, want %q", got, want)
	}
	if got, want := apiErr.RequestID, "header-request"; got != want {
		t.Fatalf("RequestID = %q, want %q", got, want)
	}
}

func TestClient_NewNilDefaultsDoNotPanic(t *testing.T) {
	c := New("", "", "", nil, nil, nil)
	if c == nil {
		t.Fatal("New() = nil, want client")
	}
	if c.client == nil {
		t.Fatal("New().client = nil, want http client")
	}
	if got, want := c.apiURL, "https://disbug.io"; got != want {
		t.Fatalf("apiURL = %q, want %q", got, want)
	}
	if c.clock == nil {
		t.Fatal("clock = nil, want default clock")
	}
}

func hitCount(mu *sync.Mutex, hits *int) int {
	mu.Lock()
	defer mu.Unlock()

	return *hits
}
