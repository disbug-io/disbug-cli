# Night Shift Notes

## What changed

- Added one README section that explains the released v0.4.1 cloud-session and local-JSON handoff paths.
- Distinguished direct `disbug configure` support (Codex, Claude Code, Cursor) from manual MCP recipes (Claude Desktop, Hermes, OpenClaw).
- Documented bounded capture facts and explicit limits without claiming automatic pickup, direct Copilot/Windsurf support, guaranteed reproduction/fixes, broad browser compatibility, or user outcomes.

## Why

The repository is the strongest current public technical proof for Disbug's agent handoff, but the existing details were spread across quickstart, MCP, and integration sections. The new section directly answers how a report reaches an agent while staying within the checked v0.4.1 launch-proof boundary.

## Verification

- Required: `go test ./...`
- Check all newly named CLI commands against v0.4.1 help/source.
- Check all named MCP tools against v0.4.1 registrations.
- Check README local links and heading anchors.
- Confirm the diff is documentation-only and does not overlap the changed hunks in open PRs #9, #11, #17, #18, or #19.
