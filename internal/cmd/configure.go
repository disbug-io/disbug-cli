package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/disbug-io/disbug-cli/internal/configure"
	"github.com/disbug-io/disbug-cli/internal/errfmt"
)

// ConfigureCmd connects Disbug to supported AI coding agents.
type ConfigureCmd struct {
	Agents    []string `name:"agent" help:"Agent to configure; repeat for more than one (codex, claude-code, cursor)."`
	Yes       bool     `short:"y" help:"Apply the displayed plan without prompting."`
	DryRun    bool     `name:"dry-run" help:"Show changes without applying them."`
	SkipMCP   bool     `name:"skip-mcp" help:"Do not configure the Disbug MCP server."`
	SkipSkill bool     `name:"skip-skill" help:"Do not install the using-disbug skill."`
	Force     bool     `help:"Replace an existing using-disbug skill not managed by Disbug."`
}

// Run detects agents, previews exact changes, and applies them after confirmation.
func (c *ConfigureCmd) Run(ctx context.Context, b bindings) error {
	profile := defaultProfile
	if b.Flags != nil && b.Flags.Profile != "" {
		profile = b.Flags.Profile
	}
	manager, err := configure.New(profile)
	if err != nil {
		return errfmt.UserFacingError{Message: err.Error(), Cause: err}
	}

	prompts := bufio.NewScanner(b.Stdin)
	agents, err := c.selectedAgents(manager, b, prompts)
	if err != nil {
		return err
	}
	skipMCP, skipSkill, err := c.selectedComponents(b, prompts)
	if err != nil {
		return err
	}
	options := configure.Options{Agents: agents, SkipMCP: skipMCP, SkipSkill: skipSkill, Force: c.Force}
	changes, err := manager.Plan(ctx, options)
	if err != nil {
		return errfmt.UserFacingError{Message: err.Error(), Cause: err}
	}
	configure.SortChanges(changes)

	if len(changes) == 0 {
		_, _ = fmt.Fprintln(b.Stdout, "Disbug is already configured for the selected agents.")
		_, _ = fmt.Fprintln(b.Stdout, "Verify with: disbug doctor")
		return nil
	}

	_, _ = fmt.Fprintln(b.Stdout, "Disbug will make these changes:")
	for _, change := range changes {
		_, _ = fmt.Fprintf(b.Stdout, "  %-12s %-5s %s %s\n", change.AgentName, change.Component, change.Action, change.Target)
	}
	if c.DryRun {
		_, _ = fmt.Fprintln(b.Stdout, "\nDry run only; no files were changed.")
		return nil
	}
	if !c.Yes {
		confirmed, promptErr := readYesNo(b, prompts, "Apply these changes?", true)
		if promptErr != nil {
			return promptErr
		}
		if !confirmed {
			_, _ = fmt.Fprintln(b.Stdout, "No changes made.")
			return nil
		}
	}

	result := manager.Apply(ctx, changes)
	writeApplyResult(b.Stdout, result)
	if len(result.Failures) > 0 {
		causes := make([]error, 0, len(result.Failures))
		for _, failure := range result.Failures {
			causes = append(causes, failure.Err)
		}
		message := "Disbug configuration is incomplete. Fix the failures above, then rerun `disbug configure`."
		return errfmt.UserFacingError{Message: message, Cause: errors.Join(causes...)}
	}
	_, _ = fmt.Fprintln(b.Stdout, "\nDisbug configuration complete.")
	_, _ = fmt.Fprintln(b.Stdout, "Restart active agent sessions so they load the MCP server and skill.")
	if profile == defaultProfile {
		_, _ = fmt.Fprintln(b.Stdout, "Verify with: disbug doctor")
	} else {
		_, _ = fmt.Fprintf(b.Stdout, "Verify with: disbug --profile %s doctor\n", profile)
	}
	return nil
}

func writeApplyResult(output io.Writer, result configure.ApplyResult) {
	_, _ = fmt.Fprintln(output, "\nConfiguration results:")
	for _, change := range result.Applied {
		_, _ = fmt.Fprintf(output, "  OK     %-12s %-5s %s\n", change.AgentName, change.Component, change.Target)
	}
	for _, failure := range result.Failures {
		change := failure.Change
		_, _ = fmt.Fprintf(output, "  FAILED %-12s %-5s %s - %s\n", change.AgentName, change.Component, change.Target, failure.Err)
	}
	_, _ = fmt.Fprintf(
		output,
		"Applied %d of %d changes; %d failed.\n",
		len(result.Applied),
		len(result.Applied)+len(result.Failures),
		len(result.Failures),
	)
}

func (c *ConfigureCmd) selectedAgents(
	manager *configure.Manager,
	b bindings,
	prompts *bufio.Scanner,
) ([]configure.AgentID, error) {
	if len(c.Agents) > 0 {
		selected, err := parseAgentValues(c.Agents)
		if err != nil {
			return nil, err
		}
		detected := map[configure.AgentID]bool{}
		for _, agent := range manager.Agents() {
			detected[agent.ID] = agent.Detected
		}
		for _, agent := range selected {
			if !detected[agent] {
				message := fmt.Sprintf("%s was not detected; install it before running configure", agent)
				return nil, errfmt.UserFacingError{Message: message}
			}
		}
		return selected, nil
	}
	detected := manager.Agents()
	values := make([]string, 0, len(detected))
	for _, agent := range detected {
		if agent.Detected {
			values = append(values, string(agent.ID))
		}
	}
	if len(values) == 0 {
		message := "No supported agents detected. Install Codex, Claude Code, or Cursor, then run `disbug configure` again."
		return nil, errfmt.UserFacingError{Message: message}
	}
	if c.Yes || c.DryRun {
		return parseAgentValues(values)
	}

	_, _ = fmt.Fprintf(b.Stdout, "Detected agents: %s\n", strings.Join(values, ", "))
	_, _ = fmt.Fprint(b.Stdout, "Agents to configure (comma-separated, Enter for all): ")
	line, err := readPromptLine(prompts)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(line) == "" {
		return parseAgentValues(values)
	}
	return parseAgentValues(strings.Split(line, ","))
}

func (c *ConfigureCmd) selectedComponents(b bindings, prompts *bufio.Scanner) (bool, bool, error) {
	if c.SkipMCP && c.SkipSkill {
		return true, true, errfmt.UsageError{Message: "--skip-mcp and --skip-skill cannot be used together"}
	}
	if c.Yes || c.DryRun {
		return c.SkipMCP, c.SkipSkill, nil
	}
	skipMCP := c.SkipMCP
	skipSkill := c.SkipSkill
	var err error
	if !c.SkipMCP {
		configureMCP, promptErr := readYesNo(b, prompts, "Configure the Disbug MCP server?", true)
		if promptErr != nil {
			return false, false, promptErr
		}
		skipMCP = !configureMCP
	}
	if !c.SkipSkill {
		installSkill, promptErr := readYesNo(b, prompts, "Install the using-disbug workflow skill?", true)
		if promptErr != nil {
			return false, false, promptErr
		}
		skipSkill = !installSkill
	}
	if skipMCP && skipSkill {
		err = errfmt.UsageError{Message: "nothing selected to configure"}
	}
	return skipMCP, skipSkill, err
}

func parseAgentValues(values []string) ([]configure.AgentID, error) {
	agents := make([]configure.AgentID, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		agent, err := configure.ParseAgent(value)
		if err != nil {
			return nil, errfmt.UsageError{Message: err.Error()}
		}
		agents = append(agents, agent)
	}
	return agents, nil
}

func readYesNo(b bindings, prompts *bufio.Scanner, prompt string, defaultYes bool) (bool, error) {
	suffix := " [Y/n]: "
	if !defaultYes {
		suffix = " [y/N]: "
	}
	_, _ = fmt.Fprint(b.Stdout, prompt+suffix)
	line, err := readPromptLine(prompts)
	if err != nil {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "":
		return defaultYes, nil
	case "y", "yes":
		return true, nil
	case "n", "no":
		return false, nil
	default:
		return false, errfmt.UsageError{Message: "answer yes or no"}
	}
}

func readPromptLine(scanner *bufio.Scanner) (string, error) {
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", err
		}
		return "", errfmt.UsageError{Message: "input ended before configuration was confirmed; rerun with --yes for non-interactive use"}
	}
	return scanner.Text(), nil
}
