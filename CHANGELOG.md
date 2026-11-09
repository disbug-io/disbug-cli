# Changelog

All notable changes to this project will be documented in this file.

Each version's section below is used as its GitHub release notes. Before tagging `vX.Y.Z`, move the `[Unreleased]` entries under a new `## [X.Y.Z] - YYYY-MM-DD` heading. The release workflow stops if that section is missing.

## [Unreleased]

## [0.4.2] - 2026-11-09

### Fixed
- `disbug watch` no longer exits on a project's seeded "Sample Bug Report" (session number 0). It skips the sample, and any session without a full report identity, with one stderr line, then keeps watching

## [0.4.1] - 2026-10-01

### Changed
- The default API and sign-in address is now `https://app.disbug.io` instead of `https://disbug.io`, across the CLI, MCP server, onboarding and login. Billing links, the Homebrew and Scoop homepage and the OpenAPI schema URL moved too

## [0.4.0] - 2026-09-17

### Added
- `disbug onboard`: one guided Developer setup. It signs you in through the browser (or `--manual` paste-back), swaps the short-lived onboarding credential for a persistent default Developer agent, picks the team's default project and configures the chosen coding agent (`--agent codex|claude-code|cursor`). `--force` replaces an existing profile
- `disbug onboard --status` and `--wait-for extension_installed|first_report_captured` print onboarding progress as JSON
- Onboarding shows the Chrome Web Store link without opening it, and the local extension authorization link only for local backends

### Changed
- Homebrew caveats, the Scoop post-install message and the root help now point to `disbug onboard`

## [0.3.0] - 2026-08-25

### Added
- Attachment filenames and IDs in CLI and MCP session/pin results
- CLI and MCP attachment downloads that return file contents directly to agents
- Explicit session and pin status updates with optional agent activity notes

## [0.2.0] - 2026-08-10

### Added
- `disbug configure` for confirmed MCP and managed `using-disbug` skill setup in Codex, Claude Code, and Cursor
- `disbug doctor` reports the health of each agent integration
- MCP server workflow instructions for MCP-only agents
- Guided follow-up messages from package installation to login, login to configure, and configure to doctor
- Best-effort update check that nudges to `brew upgrade disbug` / `scoop update disbug` when a newer release exists (stderr only, cached 24h, skipped for `mcp`/`completion`/`version`, opt out with `DISBUG_NO_UPDATE_CHECK=1`)

### Fixed
- `disbug configure` reports which changes succeeded and which failed, and restores the Codex or Claude configuration if replacing an MCP registration fails, so a machine is never left half configured without saying so
- `disbug doctor` accepts the documented manual setups that run a bare `disbug` command from `PATH`

## [0.1.0] - 2026-07-17

### Added
- CLI: `login`, `logout`, `whoami`, `doctor`, `sessions`, `session`, `pin`, `pins`, `search`, `watch`, `inspect`, `mcp`, `version`, `completion`
- MCP server with cloud read tools plus path-based `inspect_local_report`
- Multi-profile support via `--profile <name>` and `DISBUG_PROFILE` env
- Token storage at `<UserConfigDir>/disbug/<profile>.json` (0o600)
- Bulk pin syntax: `disbug pins 'https://app.disbug.io/acme/projects/2/sessions/5/?pin=1&fields=console' 'https://app.disbug.io/acme/projects/2/sessions/5/?pin=2&fields=network,events'`
- `disbug login --manual` paste-back flow for headless environments
- GoReleaser-built binaries for darwin/linux/windows x amd64/arm64
- Homebrew tap (installable as `disbug-io/tap`) and Scoop bucket (`disbug-io/scoop-bucket`)
