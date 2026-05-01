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
