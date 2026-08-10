# disbug

Disbug CLI and MCP server for AI coding agents.

A single Go binary that reads bug-report sessions from your team's Disbug instance, plus downloaded local report JSON files. Use the CLI from shell-capable agents, or connect its MCP server to an MCP-native agent.

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
disbug login                 # authenticate in the browser
disbug configure             # connect detected agents and install the workflow skill
disbug doctor                # verify login, backend, MCP, and skill setup
disbug sessions --status open
disbug session https://app.disbug.io/acme/projects/2/sessions/5/
disbug pin 'https://app.disbug.io/acme/projects/2/sessions/5/?pin=1' --fields console,network
disbug inspect ./disbug-report-example.json
```

`disbug configure` currently supports Codex, Claude Code, and Cursor. It shows the files it will change and asks for confirmation. For non-interactive setup, select an agent explicitly, for example `disbug configure --agent codex --yes`.

The installed `using-disbug` skill teaches investigation workflow and capability selection. Command syntax remains in `disbug --help` and `disbug <command> --help`; MCP tool syntax remains in each tool's schema.

### Inspect a downloaded local report

The Chrome extension can download a self-contained local report JSON when a user does not want to save a session to cloud. Inspect it without dumping screenshots or replay bytes into the terminal:

```bash
disbug inspect ./disbug-report-example.json
disbug inspect ./disbug-report-example.json --pin 2 --fields console,network
disbug inspect ./disbug-report-example.json --pin 2 --fields screenshot,replay
```

`screenshot` and `replay` fields are decoded to local cache file paths only when requested. The default summary prints pin feedback, URLs, artifact availability, and log counts.

### Use as an MCP server

Disbug exposes read-only MCP tools for cloud reports: `whoami`, `list_sessions`, `get_session`, `get_pin`, `get_pins`, `search_sessions`, and `search_pins`.

For downloaded local report JSON files, use `inspect_local_report` with a filesystem path. It returns the same lightweight summary as `disbug inspect`, and can inspect a single pin with selected fields without needing native host setup or a cloud upload.

Run `disbug configure` for Codex, Claude Code, or Cursor. Manual setup recipes and unsupported-client fallbacks are also available:

- [Claude Desktop](docs/integrations/claude-desktop.md)
- [Claude Code](docs/integrations/claude-code.md)
- [Codex](docs/integrations/codex.md)
- [Cursor](docs/integrations/cursor.md)
- [Hermes](docs/integrations/hermes.md)
- [OpenClaw](docs/integrations/openclaw.md)

### Multi-profile

```bash
disbug --profile work login
disbug --profile work configure
disbug --profile personal login
disbug --profile personal configure
```

Each configured profile points the selected agent at `disbug --profile <name> mcp`.

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
