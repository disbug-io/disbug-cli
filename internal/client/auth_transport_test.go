package client

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthTransport_InjectsHeaders(t *testing.T) {
	captured := make(chan http.Header, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured <- r.Header.Clone()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	client := &http.Client{
		Transport: &authTransport{
			base:      http.DefaultTransport,
			token:     "dba_test",
			userAgent: "disbug-cli-test",
		},
	}
	req, err := http.NewRequest(http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	resp.Body.Close()

	headers := <-captured
	if got, want := headers.Get("Authorization"), "Bearer dba_test"; got != want {
		t.Fatalf("Authorization = %q, want %q", got, want)
	}
	if got, want := headers.Get("User-Agent"), "disbug-cli-test"; got != want {
		t.Fatalf("User-Agent = %q, want %q", got, want)
	}
	if got, want := headers.Get("Accept"), "application/json"; got != want {
		t.Fatalf("Accept = %q, want %q", got, want)
	}
	if got := req.Header.Get("Authorization"); got != "" {
		t.Fatalf("original request Authorization = %q, want empty", got)
	}
	if got := req.Header.Get("User-Agent"); got != "" {
		t.Fatalf("original request User-Agent = %q, want empty", got)
	}
	if got := req.Header.Get("Accept"); got != "" {
		t.Fatalf("original request Accept = %q, want empty", got)
	}
}

func TestAuthTransport_PreservesExistingUA(t *testing.T) {
	captured := make(chan http.Header, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured <- r.Header.Clone()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	client := &http.Client{
		Transport: &authTransport{
			base:      http.DefaultTransport,
			token:     "dba_test",
			userAgent: "disbug-cli-test",
		},
	}
	req, err := http.NewRequest(http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("User-Agent", "custom")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	resp.Body.Close()

	headers := <-captured
	if got, want := headers.Get("User-Agent"), "custom"; got != want {
		t.Fatalf("User-Agent = %q, want %q", got, want)
	}
	if got, want := req.Header.Get("User-Agent"), "custom"; got != want {
		t.Fatalf("original request User-Agent = %q, want %q", got, want)
	}
	if got := req.Header.Get("Authorization"); got != "" {
		t.Fatalf("original request Authorization = %q, want empty", got)
	}
	if got := req.Header.Get("Accept"); got != "" {
		t.Fatalf("original request Accept = %q, want empty", got)
	}
}

func TestAuthTransport_NilBaseDefaultsToDefaultTransport(t *testing.T) {
	captured := make(chan http.Header, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured <- r.Header.Clone()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	client := &http.Client{
		Transport: &authTransport{
			token:     "dba_test",
			userAgent: "disbug-cli-test",
		},
	}
	req, err := http.NewRequest(http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do request with nil base: %v", err)
	}
	resp.Body.Close()

	headers := <-captured
	if got, want := headers.Get("Authorization"), "Bearer dba_test"; got != want {
		t.Fatalf("Authorization = %q, want %q", got, want)
	}
}
