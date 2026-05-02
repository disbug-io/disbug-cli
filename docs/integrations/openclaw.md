# OpenClaw

Configure OpenClaw to run Disbug as an MCP server over stdio.

## Configure

Edit:

```text
~/.openclaw/openclaw.json
```

Enable and configure the MCP adapter under `plugins.entries["mcp-adapter"]`:

```json
{
  "plugins": {
    "entries": {
      "mcp-adapter": {
        "enabled": true,
        "config": {
          "servers": [
            {
              "name": "disbug",
              "transport": "stdio",
              "command": "disbug",
              "args": ["mcp"]
            }
          ]
        }
      }
    }
  }
}
```

For a non-default profile, put the global flag before the command:

```json
{
  "plugins": {
    "entries": {
      "mcp-adapter": {
        "enabled": true,
        "config": {
          "servers": [
            {
              "name": "disbug",
              "transport": "stdio",
              "command": "disbug",
              "args": ["--profile", "work", "mcp"]
            }
          ]
        }
      }
    }
  }
}
```

Stdio servers can also include an `env` object if your OpenClaw environment needs explicit variables.

## Restart

Restart the OpenClaw gateway after editing the config:

```bash
openclaw gateway restart
```

## Verify

Confirm the adapter plugin is loaded:

```bash
openclaw plugins list
```

The default tool prefix is enabled, so Disbug tools are exposed as `disbug_whoami`, `disbug_list_sessions`, and similar names. For tool discovery, inspect the gateway logs or ask OpenClaw to use `disbug_whoami`.

## Troubleshooting

- PATH: run `which disbug`. If OpenClaw cannot find it, use the full path in `command`, commonly `/opt/homebrew/bin/disbug` or `/usr/local/bin/disbug`.
- Reload: run `openclaw gateway restart` after editing `~/.openclaw/openclaw.json`.
- Token: run `disbug login`, then restart the OpenClaw gateway.
- Profile: if you logged in with `disbug --profile work login`, use `args: ["--profile", "work", "mcp"]`.
