# Claude Desktop

Configure Claude Desktop to run Disbug as an MCP server over stdio.

## Configure

On macOS, edit:

```text
~/Library/Application Support/Claude/claude_desktop_config.json
```

On Windows, edit:

```text
%APPDATA%\Claude\claude_desktop_config.json
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

Quit Claude Desktop fully and start it again. Closing only a window may not reload MCP configuration.

## Verify

In Claude Desktop, ask Claude to use the Disbug `whoami` tool. A successful response should show the logged-in Disbug account and capabilities.

## Troubleshooting

- PATH: run `which disbug`. If Claude Desktop cannot find it, use the full path in `command`, commonly `/opt/homebrew/bin/disbug` or `/usr/local/bin/disbug`.
- Reload: fully quit and reopen Claude Desktop after editing the JSON file.
- Token: run `disbug login`, then restart Claude Desktop.
- Profile: if you logged in with `disbug --profile work login`, use `args: ["--profile", "work", "mcp"]`.
