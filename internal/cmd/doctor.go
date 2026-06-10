package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/disbug-io/disbug-cli/internal/errfmt"
	"github.com/disbug-io/disbug-cli/internal/localstore"
	"github.com/disbug-io/disbug-cli/internal/setup"
)

// DoctorCmd checks local CLI configuration and backend compatibility.
type DoctorCmd struct{}

var requiredCapabilities = []string{"search", "pin_field_selection", "scoped_session_lookup", "scoped_pin_lookup"}

// Run prints a simple human-readable health report.
func (c *DoctorCmd) Run(ctx context.Context, b bindings) error {
	printLocalDiagnostics(ctx, b)

	cli, tok, err := newAuthenticatedClient(b.Flags)
	if err != nil {
		var noToken errfmt.NoToken
		if errors.As(err, &noToken) {
			_, _ = fmt.Fprintln(b.Stdout, "cloud: not signed in - run `disbug login` to enable cloud MCP tools")
			return nil
		}
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

func printLocalDiagnostics(ctx context.Context, b bindings) {
	root, err := localstore.DefaultRoot()
	if err != nil {
		_, _ = fmt.Fprintf(b.Stdout, "local_store FAIL - %s\n", err)
		return
	}
	_, _ = fmt.Fprintf(b.Stdout, "local_store: %s\n", root)
	store, err := localstore.Open(root)
	if err != nil {
		_, _ = fmt.Fprintf(b.Stdout, "local_store FAIL - %s\n", err)
		return
	}
	defer func() {
		_ = store.Close()
	}()
	pragmas, err := store.Pragmas(ctx)
	if err != nil {
		_, _ = fmt.Fprintf(b.Stdout, "local_store_index FAIL - %s\n", err)
		return
	}
	_, _ = fmt.Fprintf(b.Stdout, "local_store_index OK - journal=%s synchronous=%s\n", pragmas.JournalMode, pragmas.Synchronous)

	home, err := os.UserHomeDir()
	if err != nil {
		_, _ = fmt.Fprintf(b.Stdout, "local_setup FAIL - %s\n", err)
		return
	}
	for _, item := range setup.ManifestDiagnostics(setup.Options{HomeDir: home}) {
		_, _ = fmt.Fprintf(b.Stdout, "native_manifest %s: %s", item.Path, item.Status)
		if item.ActualPath != "" && item.ActualPath != item.ExpectedPath {
			_, _ = fmt.Fprintf(b.Stdout, " - path=%s expected=%s", item.ActualPath, item.ExpectedPath)
		}
		_, _ = fmt.Fprintln(b.Stdout)
	}
	for _, entry := range sortedAgentEntries(setup.MCPStatuses(home)) {
		_, _ = fmt.Fprintf(b.Stdout, "mcp_%s: %s\n", entry.agent, entry.status)
	}
	for _, entry := range sortedAgentEntries(setup.SkillStatuses(home)) {
		_, _ = fmt.Fprintf(b.Stdout, "skill_%s_disbug_local: %s\n", entry.agent, entry.status)
	}
}

type agentEntry struct {
	agent  string
	status string
}

func sortedAgentEntries(items map[string]string) []agentEntry {
	entries := make([]agentEntry, 0, len(items))
	for agent, status := range items {
		entries = append(entries, agentEntry{agent: agent, status: status})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].agent < entries[j].agent })
	return entries
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
