# Cursor

Configure Cursor to run Disbug as an MCP server over stdio.

## Configure

Run:

```bash
disbug configure --agent cursor
```

This registers the MCP server in `~/.cursor/mcp.json` and installs the `using-disbug` workflow skill in the shared agent skill directory. Disbug previews both changes before applying them.

For non-interactive setup after the user has approved the displayed targets:

```bash
disbug configure --agent cursor --yes
```

### Manual MCP fallback

For project scope, edit:

```text
.cursor/mcp.json
```

For global scope, edit:

```text
~/.cursor/mcp.json
```

Add the Disbug server under `mcpServers`:

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

## Restart

Reload Cursor after editing MCP configuration. If the server does not appear, restart the editor.

## Verify

If the Cursor CLI is installed, run:

```bash
cursor-agent mcp list
cursor-agent mcp list-tools disbug
```

In the editor, verify from Cursor Settings > MCP. Then ask Cursor to use the Disbug `whoami` tool.

## Troubleshooting

- PATH: run `which disbug`. If Cursor cannot find it, use the full path in `command`, commonly `/opt/homebrew/bin/disbug` or `/usr/local/bin/disbug`.
- Repair: run `disbug configure --agent cursor`, then `disbug doctor`.
- Reload: reload or restart Cursor after editing `.cursor/mcp.json` or `~/.cursor/mcp.json`.
- Token: run `disbug login`, then reload Cursor.
- Profile: if you logged in with `disbug --profile work login`, use `args: ["--profile", "work", "mcp"]`.
