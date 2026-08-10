package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/disbug-io/disbug-cli/internal/configure"
	"github.com/disbug-io/disbug-cli/internal/errfmt"
)

// DoctorCmd checks local CLI configuration and backend compatibility.
type DoctorCmd struct{}

var requiredCapabilities = []string{"search", "pin_field_selection", "scoped_session_lookup", "scoped_pin_lookup"}

// Run prints a simple human-readable health report.
func (c *DoctorCmd) Run(ctx context.Context, b bindings) error {
	profile := defaultProfile
	if b.Flags != nil && b.Flags.Profile != "" {
		profile = b.Flags.Profile
	}
	printIntegrationStatus(ctx, b, profile)

	cli, tok, err := newAuthenticatedClient(b.Flags)
	if err != nil {
		var noToken errfmt.NoToken
		if errors.As(err, &noToken) {
			_, _ = fmt.Fprintln(b.Stdout, "cloud: not signed in - run `disbug login` to enable cloud CLI and MCP reads")
			return nil
		}
		return err
	}

	apiURL := tok.APIURL
	if apiURL == "" {
		apiURL = "https://disbug.io"
	}

	_, _ = fmt.Fprintf(b.Stdout, "profile: %s\n", profile)
	_, _ = fmt.Fprintf(b.Stdout, "api_url: %s\n", apiURL)

	me, err := cli.Me(ctx)
	if err != nil {
		_, _ = fmt.Fprintf(b.Stdout, "/api/me/ FAIL - %s\n", errfmt.Format(err))
		return err
	}

	_, _ = fmt.Fprintln(b.Stdout, "/api/me/ OK")
	_, _ = fmt.Fprintf(b.Stdout, "agent_name: %s\n", me.AgentName)
	_, _ = fmt.Fprintf(b.Stdout, "team_slug: %s\n", me.TeamSlug)
	_, _ = fmt.Fprintf(b.Stdout, "api_version: %s\n", me.APIVersion)

	missing := missingCapabilities(me)
	if len(missing) > 0 {
		message := "backend missing required capabilities: " + strings.Join(missing, ", ")
		_, _ = fmt.Fprintf(b.Stdout, "capabilities MISSING: %s\n", strings.Join(missing, ", "))
		return &errfmt.UserFacingError{Message: message}
	}

	_, _ = fmt.Fprintln(b.Stdout, "capabilities OK")
	return nil
}

func printIntegrationStatus(ctx context.Context, b bindings, profile string) {
	manager, err := configure.New(profile)
	if err != nil {
		_, _ = fmt.Fprintf(b.Stdout, "agents: unable to check - %s\n", err)
		return
	}
	statuses := manager.Statuses(ctx)
	if len(statuses) == 0 {
		_, _ = fmt.Fprintln(b.Stdout, "agents: none detected")
		_, _ = fmt.Fprintln(b.Stdout, "agent setup: run `disbug configure` after installing Codex, Claude Code, or Cursor")
		return
	}
	missing := false
	for _, status := range statuses {
		mcpState := "missing"
		if status.MCP {
			mcpState = "OK"
		}
		skillState := "missing"
		if status.Skill {
			skillState = "OK"
		}
		if !status.MCP || !status.Skill {
			missing = true
		}
		_, _ = fmt.Fprintf(b.Stdout, "agent %s: MCP %s, skill %s\n", status.AgentName, mcpState, skillState)
	}
	if missing {
		_, _ = fmt.Fprintln(b.Stdout, "agent setup: run `disbug configure` to repair missing integrations")
	} else {
		_, _ = fmt.Fprintln(b.Stdout, "agent setup: OK")
	}
}

func missingCapabilities(me interface{ HasCapability(string) bool }) []string {
	missing := make([]string, 0, len(requiredCapabilities))
	for _, capability := range requiredCapabilities {
		if !me.HasCapability(capability) {
			missing = append(missing, capability)
		}
	}

	return missing
}
