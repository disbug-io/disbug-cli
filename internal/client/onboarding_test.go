package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetOnboarding(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/onboarding/" {
			t.Fatalf("request = %s %s, want GET /api/onboarding/", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer dba_test" {
			t.Fatalf("Authorization = %q, want bearer token", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"team":{"id":1,"name":"Acme","slug":"acme"},
			"projects":[{"id":7,"name":"Website","is_default":true}],
			"default_project_id":7,
			"paths":[{"value":"agent","label":"Developer","requires_extension":true}],
			"progress":{"extension_installed":false,"agent_connected":true},
			"extension_install_url":"https://chrome.example/extension"
		}`)
	}))
	t.Cleanup(server.Close)

	apiClient := New(server.URL, "dba_test", "test", nil, server.Client(), nil)
	onboarding, err := apiClient.GetOnboarding(context.Background())
	if err != nil {
		t.Fatalf("GetOnboarding() error = %v", err)
	}
	if onboarding.Team.Name != "Acme" || len(onboarding.Projects) != 1 {
		t.Fatalf("GetOnboarding() = %#v, want team and project", onboarding)
	}
	if onboarding.DefaultProjectID == nil || *onboarding.DefaultProjectID != 7 {
		t.Fatalf("DefaultProjectID = %#v, want 7", onboarding.DefaultProjectID)
	}
	if !onboarding.Paths[0].RequiresExtension {
		t.Fatal("RequiresExtension = false, want true")
	}
}

func TestSelectOnboarding(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/onboarding/" {
			t.Fatalf("request = %s %s, want POST /api/onboarding/", r.Method, r.URL.Path)
		}
		var selection OnboardingSelection
		if err := json.NewDecoder(r.Body).Decode(&selection); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if selection.Path != "widget" || selection.ProjectID != 9 {
			t.Fatalf("selection = %#v, want widget project 9", selection)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"team":{"id":1,"name":"Acme","slug":"acme"},
			"selected_path":"widget",
			"selected_project_id":9,
			"widget_setup_url":"https://disbug.example/agent-setup/widget/9/key/"
		}`)
	}))
	t.Cleanup(server.Close)

	apiClient := New(server.URL, "dba_test", "test", nil, server.Client(), nil)
	onboarding, err := apiClient.SelectOnboarding(
		context.Background(),
		OnboardingSelection{Path: "widget", ProjectID: 9},
	)
	if err != nil {
		t.Fatalf("SelectOnboarding() error = %v", err)
	}
	if onboarding.SelectedPath != "widget" || onboarding.WidgetSetupURL == "" {
		t.Fatalf("SelectOnboarding() = %#v, want widget response", onboarding)
	}
}

func TestActivateOnboardingAgent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/onboarding/agent/" {
			t.Fatalf("request = %s %s, want POST /api/onboarding/agent/", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer dbo_setup" {
			t.Fatalf("Authorization = %q, want setup token", got)
		}
		var request struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if request.Name != "workstation" {
			t.Fatalf("name = %q, want workstation", request.Name)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"token":"dba_agent","agent_name":"workstation","team":"Acme","team_slug":"acme","created_by_email":"owner@example.com"}`)
	}))
	t.Cleanup(server.Close)

	apiClient := New(server.URL, "dbo_setup", "test", nil, server.Client(), nil)
	agent, err := apiClient.ActivateOnboardingAgent(context.Background(), "workstation")
	if err != nil {
		t.Fatalf("ActivateOnboardingAgent() error = %v", err)
	}
	if agent.Token != "dba_agent" || agent.TeamSlug != "acme" {
		t.Fatalf("ActivateOnboardingAgent() = %#v", agent)
	}
}
