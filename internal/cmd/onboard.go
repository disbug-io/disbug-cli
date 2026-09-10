package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/disbug-io/disbug-cli/internal/auth"
	"github.com/disbug-io/disbug-cli/internal/client"
	"github.com/disbug-io/disbug-cli/internal/configure"
	"github.com/disbug-io/disbug-cli/internal/errfmt"
	"github.com/disbug-io/disbug-cli/internal/token"
)

// OnboardCmd guides a user through the shortest setup path after browser authentication.
type OnboardCmd struct {
	APIURL     string `name:"api-url" env:"DISBUG_API_URL" help:"Disbug API URL (saved profile or https://disbug.io)."`
	ListenAddr string `name:"listen-addr" help:"Local listener host:port override."`
	NoBrowser  bool   `name:"no-browser" help:"Print browser URLs instead of opening them."`
	Force      bool   `help:"Overwrite an existing token profile."`
	Manual     bool   `help:"Paste the browser redirect URL locally instead of listening."`
	Status     bool   `help:"Print onboarding status as JSON without prompting or making changes."`
	ProjectID  int    `name:"project-id" help:"Project to inspect with --status or --wait-for (defaults to the team's default)."`
	WaitFor    string `name:"wait-for" help:"Wait for extension_installed or first_report_captured and print status as JSON."`
}

// Run authenticates, collects only required choices, and executes the selected setup path.
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
	if err := c.ensureLogin(ctx, b, prompts); err != nil {
		return err
	}

	apiClient, _, err := newAuthenticatedClient(b.Flags)
	if err != nil {
		return err
	}
	if err := apiClient.RequireCapability(ctx, "onboarding_setup"); err != nil {
		return err
	}

	onboarding, err := apiClient.GetOnboarding(ctx)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(b.Stdout, "\nTeam: %s\n", onboarding.Team.Name)

	path, err := chooseOnboardingPath(b, prompts, onboarding.Paths)
	if err != nil {
		return err
	}
	project, err := chooseOnboardingProject(b, prompts, onboarding.Projects, onboarding.DefaultProjectID)
	if err != nil {
		return err
	}
	if path.Value == "widget" {
		approved, promptErr := readYesNo(b, prompts, "Allow the coding agent to modify this repository and install the widget?", false)
		if promptErr != nil {
			return promptErr
		}
		if !approved {
			_, _ = fmt.Fprintln(b.Stdout, "Widget setup skipped. No repository files were changed.")
			return nil
		}
	}

	onboarding, err = apiClient.SelectOnboarding(ctx, client.OnboardingSelection{
		Path:      path.Value,
		ProjectID: project.ID,
	})
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(b.Stdout, "Selected %s for project %s (ID: %d).\n", path.Label, project.Name, project.ID)

	switch path.Value {
	case "agent":
		return c.runDeveloper(ctx, b, prompts, apiClient, onboarding)
	case "qa":
		return c.runQA(ctx, b, prompts, apiClient, onboarding)
	case "widget":
		return c.runWidget(b, onboarding)
	default:
		return &errfmt.UserFacingError{Message: fmt.Sprintf("Unsupported onboarding path %q.", path.Value)}
	}
}

func (c *OnboardCmd) ensureLogin(ctx context.Context, b bindings, prompts *bufio.Scanner) error {
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
			return promptErr
		}
		if useExisting {
			return nil
		}
	}
	if err != nil && !errors.Is(err, token.ErrProfileNotFound) {
		return fmt.Errorf("read profile: %w", err)
	}

	openLogin, err := readYesNo(b, prompts, "Open Disbug signup/login in your browser?", true)
	if err != nil {
		return err
	}
	if !openLogin {
		_, _ = fmt.Fprintln(b.Stdout, "Onboarding paused. Run `disbug onboard` when you are ready.")
		return &errfmt.UserFacingError{Message: "Disbug login was not authorized."}
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
	return login.Run(ctx, b)
}

func chooseOnboardingPath(
	b bindings,
	prompts *bufio.Scanner,
	paths []client.OnboardingPath,
) (client.OnboardingPath, error) {
	if len(paths) == 0 {
		return client.OnboardingPath{}, &errfmt.UserFacingError{Message: "Disbug returned no onboarding paths."}
	}
	labels := make([]string, 0, len(paths))
	for _, path := range paths {
		labels = append(labels, path.Label)
	}
	index, err := readNumberedChoice(b, prompts, "Choose your setup path:", labels)
	if err != nil {
		return client.OnboardingPath{}, err
	}
	return paths[index], nil
}

func chooseOnboardingProject(
	b bindings,
	prompts *bufio.Scanner,
	projects []client.OnboardingProject,
	defaultProjectID *int,
) (client.OnboardingProject, error) {
	if len(projects) == 0 {
		return client.OnboardingProject{}, &errfmt.UserFacingError{Message: "This team has no active Disbug project."}
	}
	if len(projects) == 1 {
		_, _ = fmt.Fprintf(b.Stdout, "Using project: %s\n", projects[0].Name)
		return projects[0], nil
	}

	labels := make([]string, 0, len(projects))
	defaultIndex := 0
	for index, project := range projects {
		label := project.Name
		if project.IsDefault {
			label += " (default)"
		}
		if defaultProjectID != nil && project.ID == *defaultProjectID {
			defaultIndex = index
		}
		labels = append(labels, label)
	}
	index, err := readNumberedChoiceWithDefault(b, prompts, "Choose a Disbug project:", labels, defaultIndex)
	if err != nil {
		return client.OnboardingProject{}, err
	}
	return projects[index], nil
}

func (c *OnboardCmd) runDeveloper(
	ctx context.Context,
	b bindings,
	prompts *bufio.Scanner,
	apiClient *client.Client,
	onboarding *client.Onboarding,
) error {
	if err := c.ensureExtension(ctx, b, prompts, apiClient, onboarding); err != nil {
		return err
	}

	agents, err := chooseDetectedAgents(b, prompts)
	if err != nil {
		return err
	}
	approved, err := readYesNo(b, prompts, "Configure the selected coding agent for Disbug?", true)
	if err != nil {
		return err
	}
	if !approved {
		_, _ = fmt.Fprintln(b.Stdout, "Agent configuration skipped. Run `disbug configure` to finish later.")
		return nil
	}

	configureCmd := ConfigureCmd{Agents: agents, Yes: true}
	if err := configureCmd.Run(ctx, b); err != nil {
		return err
	}
	if err := (&DoctorCmd{}).Run(ctx, b); err != nil {
		return err
	}

	_, _ = fmt.Fprintln(b.Stdout, "\nDeveloper setup complete. Capture and save your first bug with the Disbug extension.")
	return nil
}

func (c *OnboardCmd) runQA(
	ctx context.Context,
	b bindings,
	prompts *bufio.Scanner,
	apiClient *client.Client,
	onboarding *client.Onboarding,
) error {
	if err := c.ensureExtension(ctx, b, prompts, apiClient, onboarding); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(b.Stdout, "QA setup complete. Capture and save your first bug with the Disbug extension.")
	return nil
}

func (c *OnboardCmd) runWidget(
	b bindings,
	onboarding *client.Onboarding,
) error {
	if onboarding.WidgetSetupURL == "" {
		return &errfmt.UserFacingError{Message: "Disbug did not return project-specific widget instructions."}
	}

	_, _ = fmt.Fprintln(b.Stdout, "Widget installation approved. Follow the project-specific instructions:")
	_, _ = fmt.Fprintln(b.Stdout, onboarding.WidgetSetupURL)
	_, _ = fmt.Fprintln(b.Stdout, "After installation, verify the widget appears and create the first test report.")
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

	approved, err := readYesNo(b, prompts, "Open the Disbug page in the Chrome Web Store?", true)
	if err != nil {
		return err
	}
	if !approved {
		_, _ = fmt.Fprintln(b.Stdout, "Extension installation skipped. Run `disbug onboard` to resume later.")
		return &errfmt.UserFacingError{Message: "Chrome extension installation was not authorized."}
	}
	if c.NoBrowser {
		_, _ = fmt.Fprintf(b.Stdout, "Open this URL to install the extension:\n%s\n", onboarding.ExtensionInstallURL)
	} else if err := auth.Open(onboarding.ExtensionInstallURL); err != nil {
		_, _ = fmt.Fprintf(b.Stderr, "Could not open the browser: %s\nOpen this URL to install the extension:\n%s\n", err, onboarding.ExtensionInstallURL)
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

func chooseDetectedAgents(b bindings, prompts *bufio.Scanner) ([]string, error) {
	manager, err := configure.New(profileName(b.Flags))
	if err != nil {
		return nil, &errfmt.UserFacingError{Message: err.Error(), Cause: err}
	}
	detected := manager.Agents()
	values := make([]string, 0, len(detected))
	labels := make([]string, 0, len(detected))
	for _, agent := range detected {
		if agent.Detected {
			values = append(values, string(agent.ID))
			labels = append(labels, agent.Name)
		}
	}
	if len(values) == 0 {
		return nil, &errfmt.UserFacingError{
			Message: "No supported coding agent detected. Install Codex, Claude Code, or Cursor, then rerun onboarding.",
		}
	}
	if len(values) == 1 {
		_, _ = fmt.Fprintf(b.Stdout, "Detected coding agent: %s\n", labels[0])
		return values, nil
	}

	index, err := readNumberedChoice(b, prompts, "Choose the coding agent to configure:", labels)
	if err != nil {
		return nil, err
	}
	return []string{values[index]}, nil
}

func readNumberedChoice(
	b bindings,
	prompts *bufio.Scanner,
	prompt string,
	options []string,
) (int, error) {
	return readNumberedChoiceWithDefault(b, prompts, prompt, options, -1)
}

func readNumberedChoiceWithDefault(
	b bindings,
	prompts *bufio.Scanner,
	prompt string,
	options []string,
	defaultIndex int,
) (int, error) {
	_, _ = fmt.Fprintln(b.Stdout, prompt)
	for index, option := range options {
		_, _ = fmt.Fprintf(b.Stdout, "  %d. %s\n", index+1, option)
	}
	if defaultIndex >= 0 {
		_, _ = fmt.Fprintf(b.Stdout, "Choose [default %d]: ", defaultIndex+1)
	} else {
		_, _ = fmt.Fprint(b.Stdout, "Choose: ")
	}
	line, err := readPromptLine(prompts)
	if err != nil {
		return 0, err
	}
	line = strings.TrimSpace(line)
	if line == "" && defaultIndex >= 0 {
		return defaultIndex, nil
	}
	choice, err := strconv.Atoi(line)
	if err != nil || choice < 1 || choice > len(options) {
		return 0, &errfmt.UsageError{Message: fmt.Sprintf("choose a number between 1 and %d", len(options))}
	}
	return choice - 1, nil
}

func profileName(flags *RootFlags) string {
	if flags != nil && flags.Profile != "" {
		return flags.Profile
	}
	return defaultProfile
}
