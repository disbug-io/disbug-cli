# Night Shift Notes

## What changed

- Added one README section that explains the released v0.4.1 cloud-session and local-JSON handoff paths.
- Distinguished direct `disbug configure` support (Codex, Claude Code, Cursor) from manual MCP recipes (Claude Desktop, Hermes, OpenClaw).
- Documented bounded capture facts and explicit limits without claiming automatic pickup, direct Copilot/Windsurf support, guaranteed reproduction/fixes, broad browser compatibility, or user outcomes.

## Why

The repository is the strongest current public technical proof for Disbug's agent handoff, but the existing details were spread across quickstart, MCP, and integration sections. The new section directly answers how a report reaches an agent while staying within the checked v0.4.1 launch-proof boundary.

## Verification

- `go test ./...` passed on the host with Go 1.25.14; all packages passed.
- Built `./cmd/disbug` and checked `--help` for all eight newly referenced CLI paths: `login`, `configure`, `session`, `pin`, `pins`, `search`, `inspect`, and `mcp`.
- Checked all 11 named MCP tools against their current registrations.
- Checked all seven relative README links; every target exists. The added section introduces no links or fragment anchors.
- `git diff --check origin/main...HEAD` passed, and the branch changes only `README.md` plus this notes file.
- Compared with open PRs #9, #11, #17, #18, and #19. PRs #9 and #11 already conflict with current `main`; this branch adds no conflict beyond those existing conflicts. PRs #17, #18, and #19 merge cleanly with this branch, and none changes this branch's added README section.
- GitHub PR #20 is open against `main`, reports a clean merge state, and all five CI checks pass.
