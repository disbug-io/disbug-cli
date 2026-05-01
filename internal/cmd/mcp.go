package cmd

import (
	"context"

	"github.com/disbug-io/disbug-cli/internal/mcp"
)

// MCPCmd runs the MCP stdio server.
type MCPCmd struct{}

// Run starts the MCP stdio server using the selected profile.
func (c *MCPCmd) Run(ctx context.Context, b bindings) error {
	return mcp.Run(ctx, b.Flags.Profile, b.Stderr)
}
