---
name: using-disbug
description: Use Disbug reports, sessions, pins, or downloaded report JSON to investigate and verify bugs.
---

# Using Disbug

Use Disbug as evidence for reproducing, diagnosing, fixing, and verifying a reported bug.

## Choose the available interface

- If you can run shell commands and `disbug` is installed, use the CLI. Run `disbug --help` or `disbug <command> --help` when you need exact syntax.
- If you have Disbug MCP tools but no shell access, use those tools. Read their schemas for exact inputs.
- Follow the user's requested interface when they name one.

Do not assume that having the `disbug mcp` command means the current agent is connected to it. If MCP tools are unavailable, use the CLI when possible or ask the user to run `disbug configure` and restart the agent.

## Investigation workflow

1. Treat the report URL as the stable identity for a cloud report. Preserve any `?pin=` selection.
2. Start with the session summary or the selected pin's feedback. Establish what the user expected, what happened, the page, and the relevant time window.
3. Fetch only the evidence needed for the current hypothesis. Useful evidence can include console output, network activity, user actions, page state, screenshots, storage, and system context.
4. Correlate evidence before changing code. Prefer timestamps, request IDs, routes, and visible state over guesses.
5. Implement the smallest well-supported fix, then run the repository's relevant checks.
6. Verify against the original report. If new reports are arriving during a live debugging session, use Disbug's watch capability to continue from fresh evidence.

For a downloaded report JSON file, inspect the local file with the CLI or the MCP local-report capability. Local JSON inspection does not require cloud authentication.

## Efficient evidence use

Avoid loading every large evidence field by default. Begin with the session or pin overview, then request the smallest useful set of fields. Expand only when the current evidence is insufficient.

When authentication fails, use the CLI login flow if shell access is available. When agent integration is missing or stale, use `disbug configure`; use `disbug doctor` to verify the result.
