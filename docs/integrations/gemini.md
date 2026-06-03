# Gemini

Configure Gemini to run Disbug as an MCP server over stdio.

## Configure

Gemini stores MCP configuration in either of these locations:

Global:
```text
~/.gemini/settings.json
```

Alternative/Legacy:
```text
~/.gemini/config/mcp_config.json
```

Project-specific:
```text
./.gemini/settings.json
```

Add Disbug to the `mcpServers` object in your configuration file:

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

## Verify

Inside Gemini, ask it to use the Disbug `whoami` tool. A successful response should show the logged-in Disbug account and capabilities.

## Troubleshooting

- PATH: run `which disbug`. If Gemini cannot find it, use the full path in `command`, commonly `/opt/homebrew/bin/disbug` or `/usr/local/bin/disbug`.
- Reload: fully restart your Gemini session after editing the JSON file.
- Token: run `disbug login`, then restart Gemini.
- Profile: if you logged in with `disbug --profile work login`, use `args: ["--profile", "work", "mcp"]`.
