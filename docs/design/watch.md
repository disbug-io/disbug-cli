# disbug watch design

Print new cloud Disbug sessions as JSON lines on stdout. The command is intended for:

1. Agent teammates that monitor new Disbug sessions and notify a lead agent.
2. Shell users who want an `--exec` hook for inbox files, Slack handoff, or cron-style processing.

The CLI emits report notifications only. It does not fetch full report details, mark reports as read, or know how to wake a coding agent.

## Synopsis

```bash
disbug watch [--cloud-only]
             [--since=DURATION]
             [--status=STATUS] [--project=SLUG]
             [--exec=COMMAND]
             [--format=jsonl|text]
             [--poll-interval=DURATION]
```

`--cloud-only` is accepted for clarity and backwards-compatible scripts. Cloud is the only launch source.

## Filters

| Flag | Behavior |
|---|---|
| `--since=DURATION` | Backfill sessions newer than `now - DURATION` before going live. |
| `--status=STATUS` | Same semantics as `disbug sessions --status`. |
| `--project=SLUG` | Same semantics as `disbug sessions --project`. |

The startup line is written to stderr so it does not pollute the JSONL stream:

```text
watching: cloud  (since=2h, poll=30s)
```

## JSONL schema

Stdout is one JSON object per line. Flush after every write.

```json
{
  "report_url": "https://app.disbug.io/acme/projects/42/sessions/7/",
  "title": "Checkout button fails",
  "created_at": "2026-05-23T13:54:40Z",
  "pins": [
    {
      "number": 1,
      "feedback": "button overflows on mobile"
    },
    {
      "number": 2,
      "feedback": "console shows a 500"
    }
  ]
}
```

- Each line represents one new report, so no event type or source wrapper is needed.
- `pins` contains every pin's number and feedback, ordered by pin number. It intentionally excludes artifacts and metadata.
- `report_url` can be passed directly to `disbug session <report_url>` when the active agent decides to pick up the report.
- `--since` output uses the same shape. The CLI still suppresses `--exec` for those backfill lines.

## Text Format

`--format=text` produces one human-readable line per event:

```text
[NEW] Checkout button fails - button overflows on mobile (2 pins)
```

JSONL is the canonical machine contract.

## Exec Hook

`--exec=COMMAND` runs `sh -c "$COMMAND"` once per live event with:

| Variable | Value |
|---|---|
| `DISBUG_EVENT_ID` | Stable scoped report identifier |
| `DISBUG_EVENT_SOURCE` | `cloud` |
| `DISBUG_EVENT_REF` | `report_url` |
| `DISBUG_EVENT_URL` | `report_url` |
| `DISBUG_EVENT_JSON` | Full event JSON as a string |

Rules:

- Backfill events do not run `--exec`.
- Non-zero exit writes a warning to stderr and does not stop the watcher.
- Commands run serially.

## Polling

The command polls the sessions API on `--poll-interval` (default `30s`). Each successful poll advances the `created_at_after` watermark, so later polls request only sessions created during the next interval. A small in-memory ID set removes overlap duplicates.

The list request asks for title, report URL, and all pin feedbacks in one lightweight response. It never calls the full session-detail endpoint; only an explicit `disbug session <report_url>` or MCP read records an agent pickup.

## Out Of Scope

- Notifications. Consumers own notification behavior.
- Additional event types such as `session.updated` or `pin.added`.
- Server-side streaming.
- Waking an agent after its coding session has closed.
- Persistent dedupe across watcher restarts.
