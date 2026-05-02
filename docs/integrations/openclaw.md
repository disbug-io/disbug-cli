# OpenClaw

Configure OpenClaw to run Disbug as an MCP server over stdio.

## Configure

Run:

```bash
openclaw mcp set disbug '{"command":"disbug","args":["mcp"]}'
```

OpenClaw stores this under `mcp.servers`. Setting the server updates the registry; it does not validate that `disbug` is reachable.

For a non-default profile, put the global flag before the command:

```bash
openclaw mcp set disbug '{"command":"disbug","args":["--profile","work","mcp"]}'
```

## Restart

Start a new OpenClaw session, or reload MCP configuration if your OpenClaw session exposes a reload action.

## Verify

Run:

```bash
openclaw mcp list
openclaw mcp show disbug --json
```

Then ask OpenClaw to use the Disbug `whoami` tool from the MCP server.

## Troubleshooting

- PATH: run `which disbug`. If OpenClaw cannot find it, use the full path in the registry command, commonly `/opt/homebrew/bin/disbug` or `/usr/local/bin/disbug`.
- Reload: setting the registry does not prove reachability; start a new OpenClaw session or reload MCP configuration before testing.
- Token: run `disbug login`, then start a new OpenClaw session.
- Profile: if you logged in with `disbug --profile work login`, use `args: ["--profile", "work", "mcp"]`.
