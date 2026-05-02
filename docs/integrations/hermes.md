# Hermes

Configure Hermes to run Disbug as an MCP server over stdio.

## Configure

Edit:

```text
~/.hermes/config.yaml
```

Add Disbug under `mcp_servers`:

```yaml
mcp_servers:
  disbug:
    command: "disbug"
    args: ["mcp"]
```

Inline YAML is also valid:

```yaml
mcp_servers: { disbug: { command: "disbug", args: ["mcp"] } }
```

For a non-default profile, put the global flag before the command:

```yaml
mcp_servers:
  disbug:
    command: "disbug"
    args: ["--profile", "work", "mcp"]
```

## Reload

Inside Hermes, run:

```text
/reload-mcp
```

If the server still does not appear, start a new Hermes session.

## Verify

Hermes tool names use `mcp_<server>_<tool>`. Ask Hermes to use `mcp_disbug_whoami`, or ask it to list available MCP tools and confirm the Disbug tools are present.

## Troubleshooting

- PATH: run `which disbug`. If Hermes cannot find it, use the full path in `command`, commonly `/opt/homebrew/bin/disbug` or `/usr/local/bin/disbug`.
- Reload: run `/reload-mcp` after editing `~/.hermes/config.yaml`; start a new session if needed.
- Token: run `disbug login`, then run `/reload-mcp`.
- Profile: if you logged in with `disbug --profile work login`, use `args: ["--profile", "work", "mcp"]`.
