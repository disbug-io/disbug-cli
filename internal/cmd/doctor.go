package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/disbug-io/disbug-cli/internal/errfmt"
)

// DoctorCmd checks local CLI configuration and backend compatibility.
type DoctorCmd struct{}

var requiredCapabilities = []string{"search", "pin_field_selection", "pin_by_number"}

// Run prints a simple human-readable health report.
func (c *DoctorCmd) Run(ctx context.Context, b bindings) error {
	cli, tok, err := newAuthenticatedClient(b.Flags)
	if err != nil {
		return err
	}

	profile := defaultProfile
	if b.Flags != nil && b.Flags.Profile != "" {
		profile = b.Flags.Profile
	}
	apiURL := tok.APIURL
	if apiURL == "" {
		apiURL = "https://disbug.io"
	}

	_, _ = fmt.Fprintf(b.Stdout, "profile: %s\n", profile)
	_, _ = fmt.Fprintf(b.Stdout, "api_url: %s\n", apiURL)

	me, err := cli.Me(ctx)
	if err != nil {
		_, _ = fmt.Fprintf(b.Stdout, "/api/me/ FAIL - %s\n", err.Error())
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

func missingCapabilities(me interface{ HasCapability(string) bool }) []string {
	missing := make([]string, 0, len(requiredCapabilities))
	for _, capability := range requiredCapabilities {
		if !me.HasCapability(capability) {
			missing = append(missing, capability)
		}
	}

	return missing
}
