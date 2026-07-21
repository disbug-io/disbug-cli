# disbug

Disbug CLI and MCP server for AI coding agents.

A single Go binary that reads sessions from your team's Disbug instance, plus downloaded local session JSON files. Use it from your terminal, or hook it into Claude Desktop / Claude Code / Codex / OpenClaw / Hermes / Cursor as an MCP server.

## Install

Use exactly one section that matches your operating system. After installation, run `disbug version` before continuing.

### macOS / Linux (Homebrew)

```bash
brew install disbug-io/tap/disbug
disbug version
```

### Windows (Scoop)

```powershell
scoop bucket add disbug https://github.com/disbug-io/scoop-bucket
scoop install disbug
disbug version
```

## Quickstart

```bash
disbug login                 # opens a browser; saves token to <UserConfigDir>/disbug/default.json
disbug whoami                # confirms identity + capabilities
disbug sessions --status open
disbug session https://app.disbug.io/acme/projects/2/sessions/5/
disbug resolve https://app.disbug.io/acme/projects/2/sessions/5/ --summary "Fixed checkout and ran tests."
disbug pin 'https://app.disbug.io/acme/projects/2/sessions/5/?pin=1' --fields console,network
disbug inspect ./disbug-report-example.json
```

### Inspect a downloaded local session

The Chrome extension can download a self-contained local session JSON when a user does not want to save a session to the cloud. The backward-compatible filename still starts with `disbug-report-`. Inspect it without dumping screenshots or replay bytes into the terminal:

```bash
disbug inspect ./disbug-report-example.json
disbug inspect ./disbug-report-example.json --pin 2 --fields console,network
disbug inspect ./disbug-report-example.json --pin 2 --fields screenshot,replay
```

`screenshot` and `replay` fields are decoded to local cache file paths only when requested. The default summary prints pin feedback, URLs, artifact availability, and log counts.

### Use as an MCP server

Disbug exposes MCP tools for cloud sessions: `whoami`, `list_sessions`, `get_session`, `get_pin`, `get_pins`, `search_sessions`, `search_pins`, and `resolve_session`. Only `resolve_session` writes; it requires a fix-and-verification summary.

For downloaded local session JSON files, use the backward-compatible `inspect_local_report` tool with a filesystem path. It returns the same lightweight summary as `disbug inspect`, and can inspect a single pin with selected fields without needing native host setup or a cloud upload.

Agent setup recipes:

- [Claude Desktop](docs/integrations/claude-desktop.md)
- [Claude Code](docs/integrations/claude-code.md)
- [Codex](docs/integrations/codex.md)
- [Cursor](docs/integrations/cursor.md)
- [Hermes](docs/integrations/hermes.md)
- [OpenClaw](docs/integrations/openclaw.md)

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

For release and package publishing, see [docs/release.md](docs/release.md).

## License

MIT - see LICENSE.
