# disbug watch design

Stream new cloud Disbug sessions as JSON events on stdout. The command is intended for:

1. Agent teammates that monitor new Disbug sessions and notify a lead agent.
2. Shell users who want an `--exec` hook for inbox files, Slack handoff, or cron-style processing.

The CLI emits events only. It does not know about Claude, agent teams, or notifications.

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

## Event Schema

Stdout is one JSON object per line. Flush after every write.

```json
{
  "type": "session.new",
  "source": "cloud",
  "backfill": false,
  "emitted_at": "2026-05-23T13:54:43Z",
  "session": {
    "id": "7392",
    "ref": "disbug://cloud/7392",
    "url": "https://app.disbug.io/s/7392",
    "created_at": "2026-05-23T13:54:40Z",
    "status": "open",
    "project": "myapp",
    "source_url": "https://example.com/dashboard",
    "pin_count": 3,
    "pins": [
      {
        "pin_number": 1,
        "feedback": "button overflows on mobile",
        "url": "https://example.com/dashboard#pin-1"
      }
    ]
  }
}
```

- `type` is reserved for future event kinds. v1 emits `session.new`.
- `source` is always `cloud` for launch.
- `backfill` is `true` for `--since` backfill events. Consumers should not notify or run side effects for backfill events.
- `session.pins` is ordered by pin number.

## Text Format

`--format=text` produces one human-readable line per event:

```text
[NEW cloud] 7392 - button overflows on mobile (3 pins)
```

JSONL is the canonical machine contract.

## Exec Hook

`--exec=COMMAND` runs `sh -c "$COMMAND"` once per live event with:

| Variable | Value |
|---|---|
| `DISBUG_EVENT_ID` | `session.id` |
| `DISBUG_EVENT_SOURCE` | `cloud` |
| `DISBUG_EVENT_REF` | `session.ref` |
| `DISBUG_EVENT_URL` | `session.url` |
| `DISBUG_EVENT_JSON` | Full event JSON as a string |

Rules:

- Backfill events do not run `--exec`.
- Non-zero exit writes a warning to stderr and does not stop the watcher.
- Commands run serially.

## Polling

The command polls the sessions API on `--poll-interval` (default `30s`). Server-side long polling or SSE can be added later without changing the JSONL contract.

## Out Of Scope

- Notifications. Consumers own notification behavior.
- Additional event types such as `session.updated` or `pin.added`.
- Server-side streaming.
- Persistent dedupe across watcher restarts.
