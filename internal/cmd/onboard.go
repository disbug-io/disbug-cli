package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/disbug-io/disbug-cli/internal/client"
	"github.com/disbug-io/disbug-cli/internal/configure"
	"github.com/disbug-io/disbug-cli/internal/errfmt"
	"github.com/disbug-io/disbug-cli/internal/seams"
	"github.com/disbug-io/disbug-cli/internal/token"
)

// OnboardCmd guides a user through the shortest setup path after browser authentication.
type OnboardCmd struct {
	APIURL     string `name:"api-url" env:"DISBUG_API_URL" help:"Disbug API URL (saved profile or https://disbug.io)."`
	ListenAddr string `name:"listen-addr" help:"Local listener host:port override."`
	NoBrowser  bool   `name:"no-browser" help:"Print browser URLs instead of opening them."`
	Force      bool   `help:"Overwrite an existing token profile."`
	Manual     bool   `help:"Paste the browser redirect URL locally instead of listening."`
	Agent      string `name:"agent" env:"DISBUG_AGENT" help:"Coding agent to configure (codex, claude-code, cursor)."`
	Status     bool   `help:"Print onboarding status as JSON without prompting or making changes."`
	ProjectID  int    `name:"project-id" help:"Project to inspect with --status or --wait-for (defaults to the team's default)."`
	WaitFor    string `name:"wait-for" help:"Wait for extension_installed or first_report_captured and print status as JSON."`
}

type onboardingConnection struct {
	apiClient        *client.Client
	setupCredential  bool
	overwriteProfile bool
}

// Run authenticates and executes the short automated Developer setup.
func (c *OnboardCmd) Run(ctx context.Context, b bindings) error {
	if c.WaitFor != "" && c.WaitFor != "extension_installed" && c.WaitFor != "first_report_captured" {
		return errfmt.UsageError{Message: "--wait-for must be extension_installed or first_report_captured"}
	}
	if c.ProjectID < 0 || (c.ProjectID != 0 && !c.Status && c.WaitFor == "") {
		return errfmt.UsageError{Message: "--project-id must be positive and used with --status or --wait-for"}
	}
	if c.Status || c.WaitFor != "" {
		return c.runStatus(ctx, b)
	}
	if c.Manual && c.ListenAddr != "" {
		return errfmt.UsageError{Message: "--manual and --listen-addr are mutually exclusive"}
	}
	prompts := bufio.NewScanner(b.Stdin)
	agents, err := c.resolveCodingAgents(b)
	if err != nil {
		return err
	}
	connection, err := c.ensureLogin(ctx, b, prompts)
	if err != nil {
		return err
	}
	apiClient := connection.apiClient
	if !connection.setupCredential {
		if err := apiClient.RequireCapability(ctx, "onboarding_setup"); err != nil {
			return err
		}
	}

	onboarding, err := apiClient.GetOnboarding(ctx)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(b.Stdout, "\nTeam: %s\n", onboarding.Team.Name)
	project, err := defaultOnboardingProject(onboarding.Projects, onboarding.DefaultProjectID)
	if err != nil {
		return err
	}

	onboarding, err = apiClient.SelectOnboarding(ctx, client.OnboardingSelection{
		Path:      "agent",
		ProjectID: project.ID,
	})
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(b.Stdout, "Using Developer with %s.\n", project.Name)
	return c.runDeveloper(ctx, b, prompts, apiClient, onboarding, connection, agents)
}

func (c *OnboardCmd) ensureLogin(
	ctx context.Context,
	b bindings,
	prompts *bufio.Scanner,
) (onboardingConnection, error) {
	profile := profileName(b.Flags)
	existing, err := token.Read(profile)
	if c.APIURL == "" {
		c.APIURL = emptyDefault(existing.APIURL, "https://disbug.io")
	}
	sameAPI := strings.TrimRight(c.APIURL, "/") == strings.TrimRight(emptyDefault(existing.APIURL, "https://disbug.io"), "/")
	if err == nil && existing.Token != "" && !c.Force && sameAPI {
		useExisting, promptErr := readYesNo(
			b,
			prompts,
			fmt.Sprintf("Continue with the existing Disbug connection for team %s?", existing.Team),
			true,
		)
		if promptErr != nil {
			return onboardingConnection{}, promptErr
		}
		if useExisting {
			return onboardingConnection{
				apiClient: client.New(c.APIURL, existing.Token, loginUserAgent(), nil, nil, nil),
			}, nil
		}
	}
	if err != nil && !errors.Is(err, token.ErrProfileNotFound) {
		return onboardingConnection{}, fmt.Errorf("read profile: %w", err)
	}

	login := LoginCmd{
		APIURL:       c.APIURL,
		ListenAddr:   c.ListenAddr,
		NoBrowser:    c.NoBrowser,
		Manual:       c.Manual,
		prompts:      prompts,
		Force:        c.Force || (existing.Token != ""),
		suppressNext: true,
		onboarding:   true,
	}
	setupToken, err := login.acquireToken(ctx, b, resolvedAgentName(""), seams.DefaultSleeper())
	if err != nil {
		return onboardingConnection{}, err
	}
	_, _ = fmt.Fprintln(b.Stdout, "Browser authorization complete.")
	return onboardingConnection{
		apiClient:        client.New(c.APIURL, setupToken, loginUserAgent(), nil, nil, nil),
		setupCredential:  true,
		overwriteProfile: c.Force || existing.Token != "",
	}, nil
}

func defaultOnboardingProject(
	projects []client.OnboardingProject,
	defaultProjectID *int,
) (client.OnboardingProject, error) {
	if len(projects) == 0 {
		return client.OnboardingProject{}, &errfmt.UserFacingError{Message: "This team has no active Disbug project."}
	}
	if defaultProjectID != nil {
		for _, project := range projects {
			if project.ID == *defaultProjectID {
				return project, nil
			}
		}
	}
	for _, project := range projects {
		if project.IsDefault {
			return project, nil
		}
	}
	return projects[0], nil
}

func (c *OnboardCmd) runDeveloper(
	ctx context.Context,
	b bindings,
	prompts *bufio.Scanner,
	apiClient *client.Client,
	onboarding *client.Onboarding,
	connection onboardingConnection,
	agents []string,
) error {
	if connection.setupCredential {
		persistentClient, err := c.activateDeveloper(ctx, b, apiClient, connection.overwriteProfile)
		if err != nil {
			return err
		}
		apiClient = persistentClient
	}

	configureCmd := ConfigureCmd{Agents: agents, Yes: true}
	if err := configureCmd.Run(ctx, b); err != nil {
		return err
	}
	if err := (&DoctorCmd{}).Run(ctx, b); err != nil {
		return err
	}
	if err := c.ensureExtension(ctx, b, prompts, apiClient, onboarding); err != nil {
		return err
	}
	capture, err := readYesNo(
		b,
		prompts,
		"Chrome extension installed. Would you like to capture your first bug with Disbug?",
		true,
	)
	if err != nil {
		return err
	}
	if capture {
		_, _ = fmt.Fprintln(b.Stdout, "Open the Disbug extension, capture the issue, and save the report.")
	} else {
		_, _ = fmt.Fprintln(b.Stdout, "Developer setup complete. Capture your first bug whenever you are ready.")
	}
	return nil
}

func (c *OnboardCmd) ensureExtension(
	ctx context.Context,
	b bindings,
	prompts *bufio.Scanner,
	apiClient *client.Client,
	onboarding *client.Onboarding,
) error {
	if onboarding.Progress.ExtensionInstalled {
		_, _ = fmt.Fprintln(b.Stdout, "Chrome extension already detected; skipping installation.")
		return nil
	}
	if onboarding.ExtensionInstallURL == "" {
		return &errfmt.UserFacingError{Message: "The Chrome Web Store URL is not configured in Disbug."}
	}

	_, _ = fmt.Fprintf(b.Stdout, "Chrome Web Store:\n%s\n", onboarding.ExtensionInstallURL)
	if isLocalAPI(c.APIURL) && onboarding.ExtensionWelcomeURL != "" {
		_, _ = fmt.Fprintf(b.Stdout, "After installing the extension, open this local authorization page:\n%s\n", onboarding.ExtensionWelcomeURL)
	}
	_, _ = fmt.Fprintln(b.Stdout, "Waiting for Disbug to detect the authorized extension...")

	projectID := 0
	if onboarding.SelectedProjectID != nil {
		projectID = *onboarding.SelectedProjectID
	}
	if _, err := waitForOnboarding(ctx, apiClient, projectID, "extension_installed", onboardingWaitTimeout, onboardingPollInterval); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(b.Stdout, "Extension installation detected.")
	return nil
}

func (c *OnboardCmd) resolveCodingAgents(b bindings) ([]string, error) {
	if strings.TrimSpace(c.Agent) != "" {
		if _, err := configure.ParseAgent(c.Agent); err != nil {
			return nil, errfmt.UsageError{Message: err.Error()}
		}
		return []string{strings.TrimSpace(c.Agent)}, nil
	}
	manager, err := configure.New(profileName(b.Flags))
	if err != nil {
		return nil, &errfmt.UserFacingError{Message: err.Error(), Cause: err}
	}
	detected := manager.Agents()
	values := make([]string, 0, len(detected))
	for _, agent := range detected {
		if agent.Detected {
			values = append(values, string(agent.ID))
		}
	}
	if len(values) == 0 {
		return nil, &errfmt.UserFacingError{
			Message: "No supported coding agent detected. Install Codex, Claude Code, or Cursor, then rerun onboarding.",
		}
	}
	if len(values) == 1 {
		return values, nil
	}
	return nil, errfmt.UsageError{Message: "multiple coding agents detected; use --agent codex, --agent claude-code, or --agent cursor"}
}

func (c *OnboardCmd) activateDeveloper(
	ctx context.Context,
	b bindings,
	apiClient *client.Client,
	overwriteProfile bool,
) (*client.Client, error) {
	agent, err := apiClient.ActivateOnboardingAgent(ctx, resolvedAgentName(""))
	if err != nil {
		return nil, err
	}
	profile := token.Token{
		Token:          agent.Token,
		APIURL:         c.APIURL,
		AgentName:      agent.AgentName,
		Team:           agent.Team,
		TeamSlug:       agent.TeamSlug,
		CreatedByEmail: agent.CreatedByEmail,
		CreatedAt:      seams.DefaultClock().Now().UTC().Format("2006-01-02T15:04:05Z07:00"),
	}
	if err := token.Write(profileName(b.Flags), profile, overwriteProfile); err != nil {
		return nil, err
	}
	_, _ = fmt.Fprintf(b.Stdout, "Created agent %s for team %s.\n", agent.AgentName, agent.Team)
	return client.New(c.APIURL, agent.Token, loginUserAgent(), nil, nil, nil), nil
}

func isLocalAPI(rawURL string) bool {
	rawURL = strings.ToLower(strings.TrimSpace(rawURL))
	return strings.HasPrefix(rawURL, "http://localhost") || strings.HasPrefix(rawURL, "http://127.0.0.1")
}

func profileName(flags *RootFlags) string {
	if flags != nil && flags.Profile != "" {
		return flags.Profile
	}
	return defaultProfile
}
