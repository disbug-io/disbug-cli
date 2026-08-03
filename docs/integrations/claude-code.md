# Claude Code

Configure Claude Code to run Disbug as an MCP server over stdio.

## Configure

Run:

```bash
disbug configure --agent claude-code
```

This registers the MCP server at user scope and installs the `using-disbug` workflow skill in `~/.claude/skills`. Disbug previews both changes before applying them.

For non-interactive setup after the user has approved the displayed targets:

```bash
disbug configure --agent claude-code --yes
```

### Manual MCP fallback

For the user Claude Code scope, run:

```bash
claude mcp add --scope user disbug -- disbug mcp
```

Claude Code stores user-scope MCP configuration in:

```text
~/.claude.json
```

For project scope, add a `.mcp.json` file to the project with the same `mcpServers` shape:

```json
{
  "mcpServers": {
    "disbug": {
      "command": "disbug",
      "args": ["mcp"]
    }
  }
}
```

For a non-default profile, put the global flag before the command:

```json
{
  "mcpServers": {
    "disbug": {
      "command": "disbug",
      "args": ["--profile", "work", "mcp"]
    }
  }
}
```

For the user Claude Code scope with a non-default profile, run:

```bash
claude mcp add --scope user disbug -- disbug --profile work mcp
```

## Restart

Start a new Claude Code session after changing MCP configuration. If a session is already open, use `/mcp` to check whether the server is loaded.

## Verify

Run:

```bash
claude mcp list
```

Inside Claude Code, run `/mcp` and confirm `disbug` is listed. Then ask Claude Code to use the Disbug `whoami` tool.

## Troubleshooting

- PATH: run `which disbug`. If Claude Code cannot find it, use the full path in the server command, commonly `/opt/homebrew/bin/disbug` or `/usr/local/bin/disbug`.
- Repair: run `disbug configure --agent claude-code`, then `disbug doctor`.
- Reload: start a new Claude Code session after changing config, then check `/mcp`.
- Token: run `disbug login`, then start a new Claude Code session.
- Profile: if you logged in with `disbug --profile work login`, use `args: ["--profile", "work", "mcp"]`.
