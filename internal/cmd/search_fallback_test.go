package cmd

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSearchLocalFallback(t *testing.T) {
	var listSessionsCalled bool
	var searchCalled bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/me/":
			writeSearchCapability(w) // No "search" capability
		case "/api/sessions/":
			listSessionsCalled = true
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{
				"results": [
					{"id": 1, "url": "https://example.com/one", "first_pin_feedback": "matches query", "project_name": "P1"},
					{"id": 2, "url": "https://example.com/two", "first_pin_feedback": "nope", "project_name": "P2"}
				],
				"count": 2
			}`)
		case "/api/search/":
			searchCalled = true
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	setupClient(t, srv)

	stdout, stderr, err := executeSearch(t, "search", "matches")
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil; stderr=%q", err, stderr)
	}

	if !listSessionsCalled {
		t.Fatal("ListSessions was not called for local fallback")
	}
	if searchCalled {
		t.Fatal("Search endpoint was called despite missing capability")
	}

	if !strings.Contains(stdout, `"id":1`) {
		t.Errorf("stdout = %q, want id 1", stdout)
	}
	if strings.Contains(stdout, `"id":2`) {
		t.Errorf("stdout = %q, don't want id 2", stdout)
	}
}

func TestSearchLocalFallbackPinsScopeErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/me/":
			writeSearchCapability(w) // No "search" capability
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	setupClient(t, srv)

	_, stderr, err := executeSearch(t, "search", "query", "--scope", "pins")
	if err == nil {
		t.Fatal("Execute() error = nil, want error for pins scope fallback")
	}

	if !strings.Contains(stderr, "Pin search requires Disbug API capability \"search\"") {
		t.Errorf("stderr = %q, want specific error message", stderr)
	}
}
