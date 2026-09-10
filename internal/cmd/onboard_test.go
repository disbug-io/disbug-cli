package cmd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func TestOnboardQAUsesExistingLoginAndDefaultProject(t *testing.T) {
	server, selected := newOnboardingBackend(t, true)
	defer server.Close()
	writeOnboardingProfile(t, server.URL)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := Execute(
		context.Background(),
		[]string{"onboard"},
		strings.NewReader("y\n2\n"),
		&stdout,
		&stderr,
	)

	require.NoError(t, err)
	assert.Empty(t, stderr.String())
	assert.Contains(t, stdout.String(), "Choose your setup path:")
	assert.Contains(t, stdout.String(), "1. Developer")
	assert.Contains(t, stdout.String(), "2. QA / Reporter")
	assert.Contains(t, stdout.String(), "Using project: Default project")
	assert.Contains(t, stdout.String(), "Chrome extension already detected")
	assert.Contains(t, stdout.String(), "QA setup complete")
	assert.Equal(t, onboardingSelection{Path: "qa", ProjectID: 11}, <-selected)
}

func TestOnboardWidgetAsksBeforeRepositoryChanges(t *testing.T) {
	server, selected := newOnboardingBackend(t, false)
	defer server.Close()
	writeOnboardingProfile(t, server.URL)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := Execute(
		context.Background(),
		[]string{"onboard"},
		strings.NewReader("y\n3\ny\n"),
		&stdout,
		&stderr,
	)

	require.NoError(t, err)
	assert.Empty(t, stderr.String())
	assert.Contains(t, stdout.String(), "Allow the coding agent to modify this repository")
	assert.Contains(t, stdout.String(), "Widget installation approved")
	assert.Contains(t, stdout.String(), "https://disbug.example/agent-setup/widget/11/key/")
	assert.Equal(t, onboardingSelection{Path: "widget", ProjectID: 11}, <-selected)
}

func TestChooseOnboardingProjectPromptsOnlyForMultipleProjects(t *testing.T) {
	defaultID := 2
	projects := []client.OnboardingProject{
		{ID: 1, Name: "API"},
		{ID: 2, Name: "Website", IsDefault: true},
	}

	var stdout bytes.Buffer
	project, err := chooseOnboardingProject(
		bindings{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: io.Discard},
		bufioScanner("\n"),
		projects,
		&defaultID,
	)

	require.NoError(t, err)
	assert.Equal(t, 2, project.ID)
	assert.Contains(t, stdout.String(), "Choose a Disbug project:")
	assert.Contains(t, stdout.String(), "Website (default)")
}

func TestOnboardDeclinedWidgetDoesNotEnableIngestion(t *testing.T) {
	server, selected := newOnboardingBackend(t, false)
	defer server.Close()
	writeOnboardingProfile(t, server.URL)
	var stdout bytes.Buffer
	require.NoError(t, Execute(context.Background(), []string{"onboard"}, strings.NewReader("y\n3\nn\n"), &stdout, io.Discard))
	assert.Empty(t, selected)
	assert.Contains(t, stdout.String(), "Widget setup skipped")
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

func TestOnboardDoesNotReuseProfileForDifferentServer(t *testing.T) {
	writeOnboardingProfile(t, "https://old.example")
	var stdout bytes.Buffer
	err := Execute(context.Background(), []string{"onboard", "--api-url", "https://new.example"}, strings.NewReader("n\n"), &stdout, io.Discard)
	require.Error(t, err)
	assert.NotContains(t, stdout.String(), "Continue with the existing")
	assert.Equal(t, "https://old.example", readLoginProfile(t, "default").APIURL)
}

func TestOnboardBrowserCallbackContinuesIntoQA(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
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
		query.Set("token", loginTestToken)
		callback.RawQuery = query.Encode()
		resp, err := http.Get(callback.String()) //nolint:gosec,noctx // Exercise the local browser callback.
		if err != nil {
			return err
		}
		return resp.Body.Close()
	})
	defer auth.SwapBrowserOpener(previous)
	var stdout bytes.Buffer
	require.NoError(t, Execute(context.Background(), []string{"onboard", "--api-url", server.URL}, strings.NewReader("y\n2\n"), &stdout, io.Discard))
	assert.Equal(t, "qa", (<-selected).Path)
	assert.Contains(t, stdout.String(), "QA setup complete")
	assert.NotContains(t, stdout.String(), loginTestToken)
}

func TestOnboardManualSharesBufferedPromptInput(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("DISBUG_ENABLE_TEST_HOOKS", "1")
	t.Setenv("DISBUG_TEST_DETERMINISTIC_RANDOM", "onboard-manual")
	server, selected := newOnboardingBackend(t, true)
	defer server.Close()
	// The same deterministic state is generated inside manual login.
	state, err := auth.GenerateState(nil)
	require.NoError(t, err)
	input := "y\nhttp://127.0.0.1:1234/cb?token=" + loginTestToken + "&state=" + state + "\n2\n"
	var stdout bytes.Buffer
	require.NoError(t, Execute(context.Background(), []string{"onboard", "--manual", "--api-url", server.URL}, strings.NewReader(input), &stdout, io.Discard))
	assert.Equal(t, "qa", (<-selected).Path)
	assert.Contains(t, stdout.String(), "QA setup complete")
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
				"capabilities":["onboarding_setup"]
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
		"extension_install_url":"https://chrome.example/extension"` + suffix + `
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

func bufioScanner(input string) *bufio.Scanner {
	return bufio.NewScanner(strings.NewReader(input))
}
