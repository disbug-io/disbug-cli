# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

### Added
- CLI: `login`, `logout`, `whoami`, `doctor`, `sessions`, `session`, `pin`, `pins`, `search`, `mcp`, `version`, `completion`
- MCP server with 7 read-only tools: `whoami`, `list_sessions`, `get_session`, `get_pin`, `get_pins`, `search_sessions`, `search_pins`
- Multi-profile support via `--profile <name>` and `DISBUG_PROFILE` env
- Token storage at `<UserConfigDir>/disbug/<profile>.json` (0o600)
- Style A bulk pin syntax: `disbug pins 7392.2:console 7392.3:network,events`
- `disbug login --manual` paste-back flow for headless environments
- GoReleaser-built binaries for darwin/linux/windows x amd64/arm64
- Homebrew tap (`disbug-io/homebrew-tap`) and Scoop bucket (`disbug-io/scoop-bucket`)
