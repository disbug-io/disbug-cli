package cmd

import (
	"context"
	"fmt"

	"github.com/disbug-io/disbug-cli/internal/localstore"
	"github.com/disbug-io/disbug-cli/internal/outfmt"
	"github.com/disbug-io/disbug-cli/internal/setup"
)

// SetupLocalCmd installs local native-host and MCP configuration.
type SetupLocalCmd struct {
	ExtensionIDs []string `name:"extension-id" help:"Chrome extension id to allow. Repeat for production and dev ids."`
	SkipMCP      bool     `name:"skip-mcp" help:"Install browser native-host manifests without editing MCP configs."`
}

// Run installs local AI handoff support.
func (c *SetupLocalCmd) Run(ctx context.Context, b bindings) error {
	result, err := setup.Install(setup.Options{
		ExtensionIDs: c.ExtensionIDs,
		SkipMCP:      c.SkipMCP,
	})
	if err != nil {
		return err
	}
	if b.Flags != nil && b.Flags.Pretty {
		return outfmt.WriteJSON(b.Stdout, result, true)
	}
	if b.Flags == nil || !b.Flags.Verbose {
		_, _ = fmt.Fprintln(b.Stdout, "Disbug local setup complete. You can now click Copy in the Disbug extension.")
		_ = ctx
		return nil
	}
	_, _ = fmt.Fprintf(b.Stdout, "native_messaging_manifests: %d\n", len(result.Manifests))
	for _, manifest := range result.Manifests {
		_, _ = fmt.Fprintf(b.Stdout, "- %s\n", manifest)
	}
	for _, entry := range sortedAgentEntries(result.MCP) {
		_, _ = fmt.Fprintf(b.Stdout, "mcp_%s: %s\n", entry.agent, entry.status)
	}
	for _, entry := range sortedAgentEntries(result.Skills) {
		_, _ = fmt.Fprintf(b.Stdout, "skill_%s_disbug_local: %s\n", entry.agent, entry.status)
	}

	if root, err := localstore.DefaultRoot(); err == nil {
		_, _ = fmt.Fprintf(b.Stdout, "\nLocal reports will be saved to %s\n", root)
	}
	_, _ = fmt.Fprintln(b.Stdout, "Click Copy in the Disbug extension to send a report to your local AI agent.")
	_ = ctx
	return nil
}
