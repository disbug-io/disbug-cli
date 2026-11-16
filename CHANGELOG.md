# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]
### Fixed
- Profiles saved by v0.4.0 and earlier still pointed at `https://disbug.io`, so every command failed with "not found" after the API moved to `https://app.disbug.io`. The CLI and MCP server now use `https://app.disbug.io` for those profiles and update the saved file (kept at mode 0600), printing one note on stderr

### Security
- Builds now use Go 1.26.10, pinned with a `toolchain` line in `go.mod`. v0.4.2 was built with Go 1.25.0, which has known standard-library vulnerabilities. CI and the release build fail if a different Go is used, and CI now runs govulncheck

### Changed
- Updated dependencies: kong 1.17.0, MCP Go SDK 1.8.0, oapi-codegen runtime 1.8.0, kin-openapi 0.153.0, testify 1.12.1, golang.org/x/sys 0.44.0

### Fixed
- `disbug watch` no longer exits on a project's seeded "Sample Bug Report" (session number 0). It skips the sample, and any session without a full report identity, with one stderr line, then keeps watching

### Added
- Attachment filenames and IDs in CLI and MCP session/pin results
- CLI and MCP attachment downloads that return file contents directly to agents
- Explicit session and pin status updates with optional agent activity notes
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
