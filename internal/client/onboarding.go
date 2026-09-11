package client

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strconv"
)

// OnboardingProject is an active Disbug project available to the authenticated team.
type OnboardingProject struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	IsDefault bool   `json:"is_default"`
}

// OnboardingPath describes one setup path and the actions it requires.
type OnboardingPath struct {
	Value                      string `json:"value"`
	Label                      string `json:"label"`
	RequiresExtension          bool   `json:"requires_extension"`
	RequiresAgentConfiguration bool   `json:"requires_agent_configuration"`
	ModifiesRepository         bool   `json:"modifies_repository"`
}

// OnboardingProgress contains server-observable setup milestones.
type OnboardingProgress struct {
	ExtensionInstalled  bool `json:"extension_installed"`
	AgentConnected      bool `json:"agent_connected"`
	WidgetConfigured    bool `json:"widget_configured"`
	FirstReportCaptured bool `json:"first_report_captured"`
}

// Onboarding is the interactive setup contract returned by the Disbug API.
type Onboarding struct {
	Team struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
		Slug string `json:"slug"`
	} `json:"team"`
	Projects            []OnboardingProject `json:"projects"`
	DefaultProjectID    *int                `json:"default_project_id"`
	Paths               []OnboardingPath    `json:"paths"`
	SelectedPath        string              `json:"selected_path"`
	SelectedProjectID   *int                `json:"selected_project_id"`
	Progress            OnboardingProgress  `json:"progress"`
	ExtensionInstallURL string              `json:"extension_install_url"`
	ExtensionWelcomeURL string              `json:"extension_welcome_url"`
	OnboardingURL       string              `json:"onboarding_url"`
	WidgetSetupURL      string              `json:"widget_setup_url"`
}

// OnboardingAgent is the persistent Developer credential created from a setup session.
type OnboardingAgent struct {
	Token          string `json:"token"`
	AgentName      string `json:"agent_name"`
	Team           string `json:"team"`
	TeamSlug       string `json:"team_slug"`
	CreatedByEmail string `json:"created_by_email"`
}

// OnboardingSelection persists the user's path and project choice.
type OnboardingSelection struct {
	Path      string `json:"path"`
	ProjectID int    `json:"project_id"`
}

// GetOnboarding returns choices and progress for the authenticated team.
func (c *Client) GetOnboarding(ctx context.Context) (*Onboarding, error) {
	return c.GetOnboardingForProject(ctx, 0)
}

// GetOnboardingForProject reads progress for a selected project, or the default when zero.
func (c *Client) GetOnboardingForProject(ctx context.Context, projectID int) (*Onboarding, error) {
	path := "/api/onboarding/"
	if projectID > 0 {
		path += "?project_id=" + strconv.Itoa(projectID)
	}
	var onboarding Onboarding
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &onboarding); err != nil {
		return nil, err
	}

	return &onboarding, nil
}

// SelectOnboarding persists the selected setup path and project.
func (c *Client) SelectOnboarding(
	ctx context.Context,
	selection OnboardingSelection,
) (*Onboarding, error) {
	body, err := json.Marshal(selection)
	if err != nil {
		return nil, err
	}

	var onboarding Onboarding
	if err := c.doJSON(ctx, http.MethodPost, "/api/onboarding/", bytes.NewReader(body), &onboarding); err != nil {
		return nil, err
	}

	return &onboarding, nil
}

// ActivateOnboardingAgent exchanges a short-lived setup session for a persistent Developer token.
func (c *Client) ActivateOnboardingAgent(ctx context.Context, name string) (*OnboardingAgent, error) {
	body, err := json.Marshal(struct {
		Name string `json:"name"`
	}{Name: name})
	if err != nil {
		return nil, err
	}

	var agent OnboardingAgent
	if err := c.doJSON(ctx, http.MethodPost, "/api/onboarding/agent/", bytes.NewReader(body), &agent); err != nil {
		return nil, err
	}

	return &agent, nil
}
