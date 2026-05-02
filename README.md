# disbug

Disbug CLI and MCP server for AI coding agents.

A single Go binary that reads bug-report sessions from your team's Disbug instance. Use it from your terminal, or hook it into Claude Desktop / Claude Code / Codex / OpenClaw / Hermes / Cursor as an MCP server.

## Install

### macOS / Linux (Homebrew)

```bash
brew install disbug-io/tap/disbug
```

### Windows (Scoop)

```powershell
scoop bucket add disbug https://github.com/disbug-io/scoop-bucket
scoop install disbug
```

### Direct download

Grab a binary for your OS/arch from [Releases](https://github.com/disbug-io/disbug-cli/releases) and put it on your PATH.

## Quickstart

```bash
disbug login                 # opens a browser; saves token to ~/.config/disbug/default.json
disbug whoami                # confirms identity + capabilities
disbug sessions --status open
disbug session 7392
disbug pin 7392.2 --fields console,network
```

### Use as an MCP server (Claude Desktop)

Add to `~/Library/Application Support/Claude/claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "disbug": { "command": "disbug", "args": ["mcp"] }
  }
}
```

Then restart Claude Desktop. The agent now has 7 read-only tools: `whoami`, `list_sessions`, `get_session`, `get_pin`, `get_pins`, `search_sessions`, `search_pins`.

### Use as an MCP server (Claude Code)

```bash
claude mcp add --transport stdio disbug -- disbug mcp
```

### Use as an MCP server (Codex CLI / Desktop)

Edit `~/.codex/config.toml`:

```toml
[mcp_servers.disbug]
command = "disbug"
args = ["mcp"]
```

### Multi-profile

```bash
disbug --profile work login
disbug --profile personal login
```

In the agent config, add the binary twice with `args: ["--profile", "work", "mcp"]` etc.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | OK |
| 2 | Usage error (bad ref, bad flag) |
| 4 | Auth error (no token, 401) |
| 5 | Network unreachable |
| 6 | Not found (404) |
| 7 | Forbidden (403, including free-tier locked) |
| 8 | Rate limited (429) |
| 9 | Server error (5xx) |

## License

MIT - see LICENSE.
