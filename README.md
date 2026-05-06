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
disbug login                 # opens a browser; saves token to <UserConfigDir>/disbug/default.json
disbug whoami                # confirms identity + capabilities
disbug sessions --status open
disbug session 7392
disbug pin 7392.2 --fields console,network
```

### Use as an MCP server

Disbug exposes read-only MCP tools for cloud and local reports: `whoami`, `list_sessions`, `get_session`, `get_pin`, `get_pins`, `search_sessions`, `search_pins`, and `get_latest_local_session`.

Agent setup recipes:

- [Claude Desktop](docs/integrations/claude-desktop.md)
- [Claude Code](docs/integrations/claude-code.md)
- [Codex](docs/integrations/codex.md)
- [Cursor](docs/integrations/cursor.md)
- [Hermes](docs/integrations/hermes.md)
- [OpenClaw](docs/integrations/openclaw.md)

### Enable local AI handoff from the browser extension

The Chrome/Brave extension can copy reports directly into a local store that `disbug mcp` can read without uploading to Disbug cloud.

```bash
disbug setup-local --extension-id <chrome-extension-id>
```

The command installs Chrome/Brave/Chromium native messaging manifests for `io.disbug.bridge`, registers Disbug MCP where supported, and installs the `disbug-local` agent skill for Codex/Claude skill folders when present. The extension shows the exact command with its runtime extension id after the first markdown copy fallback.

Manage local reports:

```bash
disbug local-sessions list
disbug local-sessions show local_...
disbug local-sessions delete local_...
disbug local-sessions prune --older-than 30d
disbug local-sessions path
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

## Development

```bash
make install-hooks
make ci
```

The pre-commit hook runs `make fmt-check` and `make lint`.

## License

MIT - see LICENSE.
