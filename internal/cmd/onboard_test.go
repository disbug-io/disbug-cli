package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/disbug-io/disbug-cli/internal/auth"
	"github.com/disbug-io/disbug-cli/internal/client"
	"github.com/disbug-io/disbug-cli/internal/token"
)

const onboardingTestToken = "dbo_1234567890ABCDEFGHIJKLMNOPQRSTUV"

func TestOnboardUsesDeveloperAndDefaultProjectWithoutChoicePrompts(t *testing.T) {
	setupOnboardingAgent(t)
	server, selected := newOnboardingBackend(t, true)
	defer server.Close()
	writeOnboardingProfile(t, server.URL)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := Execute(
		context.Background(),
		[]string{"onboard", "--agent", "codex"},
		strings.NewReader("y\nn\n"),
		&stdout,
		&stderr,
	)

	require.NoError(t, err)
	assert.Empty(t, stderr.String())
	assert.NotContains(t, stdout.String(), "Choose your setup path:")
	assert.NotContains(t, stdout.String(), "Choose a Disbug project:")
	assert.NotContains(t, stdout.String(), "Choose the coding agent")
	assert.Contains(t, stdout.String(), "Using Developer with Default project.")
	assert.NotContains(t, stdout.String(), "(ID:")
	assert.Contains(t, stdout.String(), "Chrome extension already detected")
	assert.Contains(t, stdout.String(), "Would you like to capture your first bug")
	assert.Equal(t, onboardingSelection{Path: "agent", ProjectID: 11}, <-selected)
}

func TestDefaultOnboardingProjectUsesAPIDefaultWithoutPrompt(t *testing.T) {
	defaultID := 2
	projects := []client.OnboardingProject{
		{ID: 1, Name: "API"},
		{ID: 2, Name: "Website", IsDefault: true},
	}

	project, err := defaultOnboardingProject(projects, &defaultID)

	require.NoError(t, err)
	assert.Equal(t, 2, project.ID)
}

func TestOnboardStatusHasNoPromptsOrWrites(t *testing.T) {
	server, selected := newOnboardingBackend(t, true)
	defer server.Close()
	writeOnboardingProfile(t, server.URL)
	var stdout bytes.Buffer
	require.NoError(t, Execute(context.Background(), []string{"onboard", "--status"}, strings.NewReader(""), &stdout, io.Discard))
	var status client.Onboarding
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &status))
	assert.True(t, status.Progress.ExtensionInstalled)
	assert.Empty(t, selected)
}

func TestOnboardRejectsUnsupportedAgentBeforeLogin(t *testing.T) {
	var stdout bytes.Buffer
	err := Execute(context.Background(), []string{"onboard", "--agent", "other"}, strings.NewReader(""), &stdout, io.Discard)
	require.ErrorContains(t, err, "unsupported agent")
	assert.NotContains(t, stdout.String(), "Opening browser")
}

func TestOnboardBrowserCallbackCreatesPersistentAgentAndContinues(t *testing.T) {
	setupOnboardingAgent(t)
	t.Setenv("DISBUG_ENABLE_TEST_HOOKS", "1")
	t.Setenv("DISBUG_TEST_FAST_SLEEP", "1")
	server, selected := newOnboardingBackend(t, true)
	defer server.Close()
	previous := auth.SwapBrowserOpener(func(rawURL string) error {
		parsed, err := url.Parse(rawURL)
		if err != nil {
			return err
		}
		assert.Equal(t, "1", parsed.Query().Get("onboarding"))
		assert.NotEmpty(t, parsed.Query().Get("state"))
		callback, err := url.Parse(parsed.Query().Get("callback"))
		if err != nil {
			return err
		}
		query := callback.Query()
		query.Set("state", parsed.Query().Get("state"))
		query.Set("token", onboardingTestToken)
		callback.RawQuery = query.Encode()
		resp, err := http.Get(callback.String()) //nolint:gosec,noctx // Exercise the local browser callback.
		if err != nil {
			return err
		}
		return resp.Body.Close()
	})
	defer auth.SwapBrowserOpener(previous)
	var stdout bytes.Buffer
	require.NoError(t, Execute(
		context.Background(),
		[]string{"onboard", "--agent", "codex", "--api-url", server.URL},
		strings.NewReader("n\n"),
		&stdout,
		io.Discard,
	))
	assert.Equal(t, "agent", (<-selected).Path)
	assert.Contains(t, stdout.String(), "Browser authorization complete")
	assert.Contains(t, stdout.String(), "Created agent")
	assert.NotContains(t, stdout.String(), loginTestToken)
	assert.NotContains(t, stdout.String(), onboardingTestToken)
	profile := readLoginProfile(t, "default")
	assert.Equal(t, loginTestToken, profile.Token)
}

func TestOnboardManualSharesBufferedPromptInput(t *testing.T) {
	setupOnboardingAgent(t)
	t.Setenv("DISBUG_ENABLE_TEST_HOOKS", "1")
	t.Setenv("DISBUG_TEST_DETERMINISTIC_RANDOM", "onboard-manual")
	server, selected := newOnboardingBackend(t, true)
	defer server.Close()
	// The same deterministic state is generated inside manual login.
	state, err := auth.GenerateState(nil)
	require.NoError(t, err)
	input := "http://127.0.0.1:1234/cb?token=" + onboardingTestToken + "&state=" + state + "\nn\n"
	var stdout bytes.Buffer
	require.NoError(t, Execute(
		context.Background(),
		[]string{"onboard", "--manual", "--agent", "codex", "--api-url", server.URL},
		strings.NewReader(input),
		&stdout,
		io.Discard,
	))
	assert.Equal(t, "agent", (<-selected).Path)
	assert.Contains(t, stdout.String(), "Developer setup complete")
}

func TestWaitForOnboardingTracksProjectUntilMilestone(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "42", r.URL.Query().Get("project_id"))
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		_, _ = io.WriteString(w, `{"progress":{"first_report_captured":`+strconv.FormatBool(calls.Add(1) > 1)+`}}`)
	}))
	defer server.Close()
	apiClient := client.New(server.URL, "test-token", "test", nil, nil, nil)
	status, err := waitForOnboarding(context.Background(), apiClient, 42, "first_report_captured", time.Second, time.Millisecond)
	require.NoError(t, err)
	assert.True(t, status.Progress.FirstReportCaptured)
	assert.Equal(t, int32(2), calls.Load())
}

func TestWaitForOnboardingTimesOutAndCancels(t *testing.T) {
	server, _ := newOnboardingBackend(t, false)
	defer server.Close()
	apiClient := client.New(server.URL, "test-token", "test", nil, nil, nil)
	_, err := waitForOnboarding(context.Background(), apiClient, 0, "extension_installed", 10*time.Millisecond, time.Millisecond)
	require.ErrorContains(t, err, "Timed out waiting")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = waitForOnboarding(ctx, apiClient, 0, "extension_installed", time.Minute, time.Minute)
	require.ErrorIs(t, err, context.Canceled)
}

type onboardingSelection struct {
	Path      string `json:"path"`
	ProjectID int    `json:"project_id"`
}

func newOnboardingBackend(t *testing.T, extensionInstalled bool) (*httptest.Server, <-chan onboardingSelection) {
	t.Helper()
	selected := make(chan onboardingSelection, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/me/":
			_, _ = io.WriteString(w, `{
				"agent_name":"Codex","team":"Acme","team_slug":"acme",
				"created_by_email":"owner@example.com","api_version":"1.0.0",
				"capabilities":["onboarding_setup","search","pin_field_selection","scoped_session_lookup","scoped_pin_lookup","attachment_download"]
			}`)
		case "/api/onboarding/":
			if r.Method == http.MethodPost {
				var selection onboardingSelection
				require.NoError(t, json.NewDecoder(r.Body).Decode(&selection))
				selected <- selection
				widgetURL := ""
				if selection.Path == "widget" {
					widgetURL = `,"widget_setup_url":"https://disbug.example/agent-setup/widget/11/key/"`
				}
				_, _ = io.WriteString(w, onboardingJSON(extensionInstalled, widgetURL))
				return
			}
			_, _ = io.WriteString(w, onboardingJSON(extensionInstalled, ""))
		case "/api/onboarding/agent/":
			assert.Equal(t, "Bearer "+onboardingTestToken, r.Header.Get("Authorization"))
			var request struct {
				Name string `json:"name"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
			assert.NotEmpty(t, request.Name)
			_, _ = io.WriteString(w, `{
				"token":"`+loginTestToken+`","agent_name":"`+request.Name+`",
				"team":"Acme","team_slug":"acme","created_by_email":"owner@example.com"
			}`)
		default:
			http.NotFound(w, r)
		}
	}))
	return server, selected
}

func onboardingJSON(extensionInstalled bool, suffix string) string {
	return `{
		"team":{"id":1,"name":"Acme","slug":"acme"},
		"projects":[{"id":11,"name":"Default project","is_default":true}],
		"default_project_id":11,
		"paths":[
			{"value":"agent","label":"Developer","requires_extension":true,"requires_agent_configuration":true},
			{"value":"qa","label":"QA / Reporter","requires_extension":true},
			{"value":"widget","label":"Widget","modifies_repository":true}
		],
		"progress":{"extension_installed":` + strconv.FormatBool(extensionInstalled) + `,"agent_connected":true},
		"extension_install_url":"https://chrome.example/extension",
		"extension_welcome_url":"http://localhost:8000/extension/welcome/"` + suffix + `
	}`
}

func writeOnboardingProfile(t *testing.T, apiURL string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	require.NoError(t, token.Write("default", token.Token{
		Token:    loginTestToken,
		APIURL:   apiURL,
		Team:     "Acme",
		TeamSlug: "acme",
	}, false))
}

func setupOnboardingAgent(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	binDir := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	codexPath := filepath.Join(binDir, "codex")
	require.NoError(t, os.WriteFile(codexPath, []byte("#!/bin/sh\nexit 0\n"), 0o755))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
