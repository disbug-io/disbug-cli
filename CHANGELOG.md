# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

### Added
- CLI: `login`, `logout`, `whoami`, `configure`, `doctor`, `sessions`, `session`, `pin`, `pins`, `search`, `watch`, `inspect`, `mcp`, `version`, `completion`
- `disbug configure` for confirmed MCP and managed `using-disbug` skill setup in Codex, Claude Code, and Cursor
- MCP server with cloud read tools plus path-based `inspect_local_report`, including workflow instructions for MCP-only agents
- Guided follow-up messages from package installation to login, login to configure, and configure to doctor
- Best-effort update check that nudges to `brew upgrade disbug` / `scoop update disbug` when a newer release exists (stderr only, cached 24h, skipped for `mcp`/`completion`/`version`, opt out with `DISBUG_NO_UPDATE_CHECK=1`)
- Multi-profile support via `--profile <name>` and `DISBUG_PROFILE` env
- Token storage at `<UserConfigDir>/disbug/<profile>.json` (0o600)
- Bulk pin syntax: `disbug pins 'https://app.disbug.io/acme/projects/2/sessions/5/?pin=1&fields=console' 'https://app.disbug.io/acme/projects/2/sessions/5/?pin=2&fields=network,events'`
- `disbug login --manual` paste-back flow for headless environments
- GoReleaser-built binaries for darwin/linux/windows x amd64/arm64
- Homebrew tap (installable as `disbug-io/tap`) and Scoop bucket (`disbug-io/scoop-bucket`)
