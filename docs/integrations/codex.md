# Codex

Configure Codex to run Disbug as an MCP server over stdio.

## Configure

Edit:

```text
~/.codex/config.toml
```

Add:

```toml
[mcp_servers.disbug]
command = "disbug"
args = ["mcp"]
```

For a non-default profile, put the global flag before the command:

```toml
[mcp_servers.disbug]
command = "disbug"
args = ["--profile", "work", "mcp"]
```

## Restart

Start a new Codex session after editing `~/.codex/config.toml`.

## Verify

Inside a Codex session, run `/mcp` and confirm `disbug` is loaded. Then ask Codex to use the Disbug `whoami` tool.

## Troubleshooting

- PATH: run `which disbug`. If Codex cannot find it, use the full path in `command`, commonly `/opt/homebrew/bin/disbug` or `/usr/local/bin/disbug`.
- Reload: start a new Codex session after editing `~/.codex/config.toml`.
- Token: run `disbug login`, then start a new Codex session.
- Profile: if you logged in with `disbug --profile work login`, use `args = ["--profile", "work", "mcp"]`.
