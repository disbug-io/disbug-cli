package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/disbug-io/disbug-cli/internal/auth"
	"github.com/disbug-io/disbug-cli/internal/errfmt"
	"github.com/disbug-io/disbug-cli/internal/token"
)

const loginTestToken = "dba_aaaaaaaaaaaaaaaaaaaaaaaa"

func TestLoginBrowserFlowPersistsTokenProfileMetadata(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("DISBUG_ENABLE_TEST_HOOKS", "1")
	t.Setenv("DISBUG_TEST_FAST_SLEEP", "1")
	t.Setenv("DISBUG_TEST_DETERMINISTIC_RANDOM", "login-browser")
	t.Setenv("DISBUG_TEST_FROZEN_TIME", "2026-01-02T03:04:05Z")

	backend := newLoginBackend(t)
	defer backend.Close()

	opened := make(chan string, 1)
	restore := auth.SwapBrowserOpener(func(rawURL string) error {
		opened <- rawURL
		return nil
	})
	defer restore()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- Execute(
			context.Background(),
			[]string{"login", "--api-url", backend.URL, "--name", "cli-test-agent"},
			strings.NewReader(""),
			&stdout,
			&stderr,
		)
	}()

	var authURL string
	select {
	case authURL = <-opened:
	case <-time.After(2 * time.Second):
		t.Fatal("browser opener was not called")
	}

	resp, err := http.Get(authURL) //nolint:gosec,noctx // test drives the browser callback redirect.
	require.NoError(t, err)
	_ = resp.Body.Close()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("login command did not finish")
	}

	assert.Contains(t, stdout.String(), "Logged in")
	assert.Contains(t, stderr.String(), "Opening")

	profile := readLoginProfile(t, "default")
	assert.Equal(t, loginTestToken, profile.Token)
	assert.Equal(t, backend.URL, profile.APIURL)
	assert.Equal(t, "cli-test-agent", profile.AgentName)
	assert.Equal(t, "QA Team", profile.Team)
	assert.Equal(t, "qa-team", profile.TeamSlug)
	assert.Equal(t, "owner@example.com", profile.CreatedByEmail)
	assert.Equal(t, "2026-01-02T03:04:05Z", profile.CreatedAt)
}

func TestLoginRespectsListenAddr(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("DISBUG_ENABLE_TEST_HOOKS", "1")
	t.Setenv("DISBUG_TEST_FAST_SLEEP", "1")
	t.Setenv("DISBUG_TEST_DETERMINISTIC_RANDOM", "login-listen-addr")

	backend := newLoginBackend(t)
	defer backend.Close()
	listenAddr := freeListenAddr(t)

	opened := make(chan string, 1)
	restore := auth.SwapBrowserOpener(func(rawURL string) error {
		opened <- rawURL
		return nil
	})
	defer restore()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- Execute(
			context.Background(),
			[]string{"login", "--api-url", backend.URL, "--listen-addr", listenAddr},
			strings.NewReader(""),
			&stdout,
			&stderr,
		)
	}()

	var authURL string
	select {
	case authURL = <-opened:
	case <-time.After(2 * time.Second):
		t.Fatal("browser opener was not called")
	}

	parsed, err := url.Parse(authURL)
	require.NoError(t, err)
	callback := parsed.Query().Get("callback")
	require.Contains(t, callback, listenAddr)

	resp, err := http.Get(authURL) //nolint:gosec,noctx // test drives the browser callback redirect.
	require.NoError(t, err)
	_ = resp.Body.Close()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("login command did not finish")
	}
}

func TestLoginTokenFromStdinPersistsToken(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("DISBUG_ENABLE_TEST_HOOKS", "1")
	t.Setenv("DISBUG_TEST_FROZEN_TIME", "2026-01-02T03:04:05Z")

	backend := newLoginBackend(t)
	defer backend.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := Execute(
		context.Background(),
		[]string{"login", "--api-url", backend.URL, "--token-from-stdin", "--name", "stdin-agent"},
		strings.NewReader("  "+loginTestToken+"  \nignored\n"),
		&stdout,
		&stderr,
	)

	require.NoError(t, err)
	assert.Contains(t, stderr.String(), "warning")
	assert.Contains(t, stdout.String(), "Logged in")

	profile := readLoginProfile(t, "default")
	assert.Equal(t, loginTestToken, profile.Token)
	assert.Equal(t, "cli-test-agent", profile.AgentName)
}

func TestLoginExistingProfileRequiresForce(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("DISBUG_ENABLE_TEST_HOOKS", "1")

	backend := newLoginBackend(t)
	defer backend.Close()
	require.NoError(t, token.Write("default", token.Token{Token: "dba_existingexistingexistingexis"}, false))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := Execute(
		context.Background(),
		[]string{"login", "--api-url", backend.URL, "--token", loginTestToken},
		strings.NewReader(""),
		&stdout,
		&stderr,
	)

	require.Error(t, err)
	var userFacing errfmt.UserFacingError
	assert.True(t, errors.As(err, &userFacing))
	assert.Contains(t, stderr.String(), "--force")
	assert.Contains(t, stderr.String(), "--profile")
	assert.Empty(t, stdout.String())
}

func TestLoginManualAndTokenAreMutuallyExclusive(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := Execute(
		context.Background(),
		[]string{"login", "--manual", "--token", loginTestToken},
		strings.NewReader(""),
		&stdout,
		&stderr,
	)

	require.Error(t, err)
	var usage errfmt.UsageError
	assert.True(t, errors.As(err, &usage))
	assert.Equal(t, 2, ExitCode(err))
	assert.Contains(t, stderr.String(), "mutually exclusive")
	assert.Empty(t, stdout.String())
}

func TestLoginTokenFromEnvEmptyReturnsUsageError(t *testing.T) {
	t.Setenv("DISBUG_LOGIN_TOKEN", "   ")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := Execute(
		context.Background(),
		[]string{"login", "--token-from-env"},
		strings.NewReader(""),
		&stdout,
		&stderr,
	)

	require.Error(t, err)
	var usage errfmt.UsageError
	assert.True(t, errors.As(err, &usage))
	assert.Equal(t, 2, ExitCode(err))
	assert.Contains(t, stderr.String(), "DISBUG_LOGIN_TOKEN")
	assert.Empty(t, stdout.String())
}

func TestLoginBrowserFlowUsesFastSleeperHook(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("DISBUG_ENABLE_TEST_HOOKS", "1")
	t.Setenv("DISBUG_TEST_FAST_SLEEP", "1")
	t.Setenv("DISBUG_TEST_DETERMINISTIC_RANDOM", "login-fast-sleep")

	backend := newLoginBackend(t)
	defer backend.Close()

	opened := make(chan string, 1)
	restore := auth.SwapBrowserOpener(func(rawURL string) error {
		opened <- rawURL
		return nil
	})
	defer restore()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	done := make(chan error, 1)
	start := time.Now()
	go func() {
		done <- Execute(
			context.Background(),
			[]string{"login", "--api-url", backend.URL},
			strings.NewReader(""),
			&stdout,
			&stderr,
		)
	}()

	var authURL string
	select {
	case authURL = <-opened:
	case <-time.After(2 * time.Second):
		t.Fatal("browser opener was not called")
	}
	resp, err := http.Get(authURL) //nolint:gosec,noctx // test drives the browser callback redirect.
	require.NoError(t, err)
	_ = resp.Body.Close()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(300 * time.Millisecond):
		t.Fatal("login command slept for real time instead of using the test sleeper hook")
	}
	assert.Less(t, time.Since(start), 300*time.Millisecond)
}

func newLoginBackend(t *testing.T) *httptest.Server {
	t.Helper()

	var meHits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/agent-auth/":
			callback := r.URL.Query().Get("callback")
			require.NotEmpty(t, callback)
			require.NotEmpty(t, r.URL.Query().Get("state"))
			redirect := callback + "?token=" + loginTestToken + "&state=" + url.QueryEscape(r.URL.Query().Get("state"))
			http.Redirect(w, r, redirect, http.StatusFound)
		case "/api/me/":
			meHits.Add(1)
			assert.Equal(t, "Bearer "+loginTestToken, r.Header.Get("Authorization"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{
				"agent_name":"cli-test-agent",
				"team":"QA Team",
				"team_slug":"qa-team",
				"created_by_email":"owner@example.com",
				"token_prefix":"dba_aaaa",
				"last_used_at":"2026-01-02T03:04:05Z",
				"api_version":"v1",
				"capabilities":["sessions:read"]
			}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(func() {
		assert.Greater(t, meHits.Load(), int64(0), "login should validate the token with /api/me/")
	})

	return server
}

func readLoginProfile(t *testing.T, name string) token.Token {
	t.Helper()

	path, err := token.ProfilePath(name)
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Clean(path))
	require.NoError(t, err)

	var profile token.Token
	require.NoError(t, json.Unmarshal(data, &profile))
	return profile
}

func freeListenAddr(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	addr, ok := listener.Addr().(*net.TCPAddr)
	require.True(t, ok)
	return fmt.Sprintf("127.0.0.1:%d", addr.Port)
}
