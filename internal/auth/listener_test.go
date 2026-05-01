package auth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/disbug-io/disbug-cli/internal/seams"
)

const validToken = "dba_1234567890ABCDEFGHIJKLMN"

func TestListenerHappyPath(t *testing.T) {
	listener, err := NewListener("STATE123", []byte("ok"), []byte("err"), "", nil, nil)
	if err != nil {
		t.Fatalf("NewListener() error = %v, want nil", err)
	}
	t.Cleanup(func() {
		_ = listener.Close()
	})

	resp := getCallback(t, listener, validToken, "STATE123")
	body := readBody(t, resp)
	if got, want := resp.StatusCode, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d, body %q", got, want, body)
	}
	if got, want := body, "ok"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}

	result, err := listener.Wait(context.Background(), time.Second)
	if err != nil {
		t.Fatalf("Wait() error = %v, want nil", err)
	}
	if result.Err != nil {
		t.Fatalf("Wait() result.Err = %v, want nil", result.Err)
	}
	if got, want := result.Token, validToken; got != want {
		t.Fatalf("Token = %q, want %q", got, want)
	}
	if got, want := result.State, "STATE123"; got != want {
		t.Fatalf("State = %q, want %q", got, want)
	}
}

func TestListenerStateMismatchReturnsResultError(t *testing.T) {
	listener, err := NewListener("STATE123", []byte("ok"), []byte("err"), "", nil, nil)
	if err != nil {
		t.Fatalf("NewListener() error = %v, want nil", err)
	}
	t.Cleanup(func() {
		_ = listener.Close()
	})

	resp := getCallback(t, listener, validToken, "WRONG")
	body := readBody(t, resp)
	if got, want := resp.StatusCode, http.StatusBadRequest; got != want {
		t.Fatalf("status = %d, want %d, body %q", got, want, body)
	}
	if got, want := body, "err"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}

	result, err := listener.Wait(context.Background(), time.Second)
	if err != nil {
		t.Fatalf("Wait() error = %v, want nil", err)
	}
	if result.Err == nil || result.Err.Error() != "state mismatch" {
		t.Fatalf("Wait() result.Err = %v, want state mismatch", result.Err)
	}
}

func TestListenerInvalidTokenReturnsResultError(t *testing.T) {
	listener, err := NewListener("STATE123", []byte("ok"), []byte("err"), "", nil, nil)
	if err != nil {
		t.Fatalf("NewListener() error = %v, want nil", err)
	}
	t.Cleanup(func() {
		_ = listener.Close()
	})

	resp := getCallback(t, listener, "bad", "STATE123")
	body := readBody(t, resp)
	if got, want := resp.StatusCode, http.StatusBadRequest; got != want {
		t.Fatalf("status = %d, want %d, body %q", got, want, body)
	}
	if got, want := body, "err"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}

	result, err := listener.Wait(context.Background(), time.Second)
	if err != nil {
		t.Fatalf("Wait() error = %v, want nil", err)
	}
	if result.Err == nil || result.Err.Error() != "invalid token format" {
		t.Fatalf("Wait() result.Err = %v, want invalid token format", result.Err)
	}
}

func TestListenerCallbackResponseHeaders(t *testing.T) {
	listener, err := NewListener("STATE123", []byte("ok"), []byte("err"), "", nil, nil)
	if err != nil {
		t.Fatalf("NewListener() error = %v, want nil", err)
	}
	t.Cleanup(func() {
		_ = listener.Close()
	})

	resp := getCallback(t, listener, validToken, "STATE123")
	_ = readBody(t, resp)
	if got, want := resp.Header.Get("Referrer-Policy"), "no-referrer"; got != want {
		t.Fatalf("Referrer-Policy = %q, want %q", got, want)
	}
	if got, want := resp.Header.Get("Cache-Control"), "no-store, max-age=0"; got != want {
		t.Fatalf("Cache-Control = %q, want %q", got, want)
	}
	if got, want := resp.Header.Get("Content-Type"), "text/html; charset=utf-8"; got != want {
		t.Fatalf("Content-Type = %q, want %q", got, want)
	}
}

func TestListenerFixedAddress(t *testing.T) {
	port, err := BindFreePort()
	if err != nil {
		t.Fatalf("BindFreePort() error = %v, want nil", err)
	}

	listener, err := NewListener("STATE123", []byte("ok"), []byte("err"), fmt.Sprintf("127.0.0.1:%d", port), nil, nil)
	if err != nil {
		t.Fatalf("NewListener() error = %v, want nil", err)
	}
	t.Cleanup(func() {
		_ = listener.Close()
	})
	if got := listener.Port(); got != port {
		t.Fatalf("Port() = %d, want %d", got, port)
	}

	resp := getCallback(t, listener, validToken, "STATE123")
	_ = readBody(t, resp)
	result, err := listener.Wait(context.Background(), time.Second)
	if err != nil {
		t.Fatalf("Wait() error = %v, want nil", err)
	}
	if result.Err != nil {
		t.Fatalf("Wait() result.Err = %v, want nil", result.Err)
	}
}

func TestListenerFactoryIsHonored(t *testing.T) {
	var requestedAddr string
	factory := seams.ListenerFactory(func(addr string) (net.Listener, error) {
		requestedAddr = addr
		return net.Listen("tcp", addr)
	})

	listener, err := NewListener("STATE123", []byte("ok"), []byte("err"), "127.0.0.1:0", factory, nil)
	if err != nil {
		t.Fatalf("NewListener() error = %v, want nil", err)
	}
	t.Cleanup(func() {
		_ = listener.Close()
	})
	if got, want := requestedAddr, "127.0.0.1:0"; got != want {
		t.Fatalf("factory addr = %q, want %q", got, want)
	}
}

func TestListenerWaitTimeoutReturnsError(t *testing.T) {
	listener, err := NewListener("STATE123", []byte("ok"), []byte("err"), "", nil, nil)
	if err != nil {
		t.Fatalf("NewListener() error = %v, want nil", err)
	}
	t.Cleanup(func() {
		_ = listener.Close()
	})

	_, err = listener.Wait(context.Background(), 10*time.Millisecond)
	if err == nil {
		t.Fatal("Wait() error = nil, want timeout error")
	}
}

func TestListenerCloseUnblocksWaitWithoutTimeout(t *testing.T) {
	listener, err := NewListener("STATE123", []byte("ok"), []byte("err"), "", nil, nil)
	if err != nil {
		t.Fatalf("NewListener() error = %v, want nil", err)
	}
	t.Cleanup(func() {
		_ = listener.Close()
	})

	errCh := make(chan error, 1)
	go func() {
		_, err := listener.Wait(context.Background(), 0)
		errCh <- err
	}()

	if err := listener.Close(); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}

	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			t.Fatalf("Wait() error = %v, want http.ErrServerClosed", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Wait() did not return after Close()")
	}
}

func TestBuildAuthURL(t *testing.T) {
	got := BuildAuthURL("https://api.example.test/", "http://127.0.0.1:1234/cb", "STATE 123", "My CLI")
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("BuildAuthURL() produced invalid URL %q: %v", got, err)
	}
	if got, want := parsed.String()[:len("https://api.example.test/agent-auth/")], "https://api.example.test/agent-auth/"; got != want {
		t.Fatalf("auth URL prefix = %q, want %q", got, want)
	}
	query := parsed.Query()
	if got, want := query.Get("callback"), "http://127.0.0.1:1234/cb"; got != want {
		t.Fatalf("callback = %q, want %q", got, want)
	}
	if got, want := query.Get("state"), "STATE 123"; got != want {
		t.Fatalf("state = %q, want %q", got, want)
	}
	if got, want := query.Get("name"), "My CLI"; got != want {
		t.Fatalf("name = %q, want %q", got, want)
	}

	got = BuildAuthURL("https://api.example.test/", "cb", "state", "")
	parsed, err = url.Parse(got)
	if err != nil {
		t.Fatalf("BuildAuthURL() without name produced invalid URL %q: %v", got, err)
	}
	if _, ok := parsed.Query()["name"]; ok {
		t.Fatalf("name query was present in %q", got)
	}
}

func TestParsePastebackURL(t *testing.T) {
	token, state, err := ParsePastebackURL("  disbug://auth?token=abc&state=STATE123  ")
	if err != nil {
		t.Fatalf("ParsePastebackURL() error = %v, want nil", err)
	}
	if got, want := token, "abc"; got != want {
		t.Fatalf("token = %q, want %q", got, want)
	}
	if got, want := state, "STATE123"; got != want {
		t.Fatalf("state = %q, want %q", got, want)
	}
}

func TestParsePastebackURLMissingValues(t *testing.T) {
	tests := []string{
		"disbug://auth?state=STATE123",
		"disbug://auth?token=abc",
	}
	for _, tt := range tests {
		t.Run(tt, func(t *testing.T) {
			_, _, err := ParsePastebackURL(tt)
			if err == nil {
				t.Fatal("ParsePastebackURL() error = nil, want error")
			}
		})
	}
}

func TestDefaultOpenerReturnsOpener(t *testing.T) {
	if DefaultOpener() == nil {
		t.Fatal("DefaultOpener() = nil, want opener")
	}
}

func getCallback(t *testing.T, listener *Listener, token, state string) *http.Response {
	t.Helper()

	callbackURL := fmt.Sprintf("http://127.0.0.1:%d/cb?token=%s&state=%s", listener.Port(), url.QueryEscape(token), url.QueryEscape(state))
	resp, err := http.Get(callbackURL)
	if err != nil {
		t.Fatalf("GET %s failed: %v", callbackURL, err)
	}

	return resp
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}

	return strings.TrimSpace(string(body))
}
