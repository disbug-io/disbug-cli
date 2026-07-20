# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

### Changed
- `disbug watch` now polls incrementally, prints a compact report URL/title/all-pin-feedback JSONL record, and no longer records an agent pickup while checking for new reports.

### Added
- CLI: `login`, `logout`, `whoami`, `doctor`, `sessions`, `session`, `pin`, `pins`, `search`, `mcp`, `version`, `completion`
- MCP server with 7 read-only tools: `whoami`, `list_sessions`, `get_session`, `get_pin`, `get_pins`, `search_sessions`, `search_pins`
- Multi-profile support via `--profile <name>` and `DISBUG_PROFILE` env
- Token storage at `<UserConfigDir>/disbug/<profile>.json` (0o600)
- Bulk pin syntax: `disbug pins 'https://app.disbug.io/acme/projects/2/sessions/5/?pin=1&fields=console' 'https://app.disbug.io/acme/projects/2/sessions/5/?pin=2&fields=network,events'`
- `disbug login --manual` paste-back flow for headless environments
- GoReleaser-built binaries for darwin/linux/windows x amd64/arm64
- Homebrew tap (installable as `disbug-io/tap`) and Scoop bucket (`disbug-io/scoop-bucket`)
