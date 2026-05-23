# disbug watch — design spec

Stream new Disbug sessions as JSON events on stdout. Designed for two consumers:

1. **Claude Code watcher teammate** — `Monitor` reads each JSONL line as an event, agent triages, then `SendMessage` to the lead.
2. **Standalone shell users** — `--exec` hook for any other action (inbox file, Slack, cron handoff).

The CLI emits events; it does **not** know about Claude, agent teams, or notifications. That separation is intentional — `disbug watch` stays useful to non-Claude consumers, and Claude integration is composed from primitives the harness already provides (`Monitor`, `SendMessage`).

## Synopsis

```
disbug watch [--local-only | --cloud-only]
             [--since=DURATION]
             [--status=STATUS] [--project=SLUG]
             [--exec=COMMAND]
             [--format=jsonl|text]
             [--poll-interval=DURATION]
```

## Source selection

| Flag | Behaviour |
|---|---|
| (none) | Auto: local + cloud if `disbug whoami` succeeds, else local-only. |
| `--local-only` | Watch localstore only. Works offline. No login required. |
| `--cloud-only` | Watch cloud API only. Exits with code 2 if not signed in. |

Startup line to stderr (so it doesn't pollute the JSONL stream):

```
watching: local, cloud  (since=2h, poll=30s)
```

## Filters

| Flag | Behaviour |
|---|---|
| `--since=DURATION` | Backfill sessions newer than `now - DURATION` before going live. Go duration syntax restricted to `s` / `m` / `h` (no `d` / `w`). Sanity cap `8760h` (1 year) so a typo doesn't backfill the universe. |
| `--status=STATUS` | Same semantics as `disbug sessions --status`. |
| `--project=SLUG` | Same semantics as `disbug sessions --project`. |

`--since` is **also added to `disbug sessions` and `disbug local-sessions list`** with the same parser. `watch` reuses those code paths for its backfill phase — no duplicated query logic.

## Output

Stdout is the event stream. One JSON object per line. Flush after every write (`grep --line-buffered`-friendly).

### Event schema (JSONL)

```json
{
  "type": "session.new",
  "source": "local",
  "backfill": false,
  "emitted_at": "2026-05-23T13:54:43Z",
  "session": {
    "id": "ses_abc123",
    "ref": "disbug://local/ses_abc123",
    "url": "https://app.disbug.io/s/ses_abc123",
    "created_at": "2026-05-23T13:54:40Z",
    "status": "new",
    "project": "myapp",
    "source_url": "https://example.com/dashboard",
    "pin_count": 3,
    "pins": [
      {
        "pin_number": 1,
        "feedback": "button overflows on mobile",
        "url": "https://example.com/dashboard#pin-1"
      },
      {
        "pin_number": 2,
        "feedback": "form submits twice on double-click",
        "url": "https://example.com/dashboard#pin-2"
      },
      {
        "pin_number": 3,
        "feedback": "logo is blurry on retina",
        "url": "https://example.com/dashboard#pin-3"
      }
    ]
  }
}
```

- `type` — reserved for future event kinds. v1 emits `session.new` only.
- `source` — `"local"` or `"cloud"`.
- `backfill` — `true` if from `--since` backfill phase. Consumers MUST treat backfill events as informational; no notifications, no `--exec`.
- `session.url` — present for cloud; omitted for local-only sessions that have no public URL.
- `session.project` — omitted for local.
- `session.pins` — full array, ordered by `pin_number` ascending. Empty array `[]` if the session has no pins (rare, but possible mid-capture). The earlier-spec `first_pin_*` flat fields are removed — consumers read `pins[0]` if they want that semantic.

### Text format

`--format=text` produces one human-readable line per event:

```
[NEW local] ses_abc123 — button overflows on mobile (3 pins)
```

For interactive use only. **JSONL is the canonical machine contract.**

## Exec hook

`--exec=COMMAND` spawns `sh -c "$COMMAND"` **once per event**, with these env vars set:

| Variable | Value |
|---|---|
| `DISBUG_EVENT_ID` | `session.id` |
| `DISBUG_EVENT_SOURCE` | `local` or `cloud` |
| `DISBUG_EVENT_REF` | `session.ref` |
| `DISBUG_EVENT_URL` | `session.url` (empty for local) |
| `DISBUG_EVENT_JSON` | Full event JSON as a string |

Rules:

- One event = one invocation. Always single-event shape, never an array.
- Non-zero exit emits a warning to stderr but does not stop the watcher.
- Commands run serially per source (no concurrent invocations); a slow exec creates back-pressure on its own source's event emission, not on the other source.

Examples:

```bash
# Append refs to an inbox file
disbug watch --exec='echo "$DISBUG_EVENT_REF" >> ~/.disbug/inbox.txt'

# Pipe full JSON into a triage script
disbug watch --exec='./scripts/triage.sh "$DISBUG_EVENT_JSON"'
```

## Backfill rule

1. If `--since` is set, query backfill via the same path `disbug sessions --since=X` (cloud) and `disbug local-sessions list --since=X` (local) use.
2. Emit each backfilled session as JSONL with `"backfill": true`, in chronological order (oldest first).
3. Record each ID in an in-memory dedupe set keyed by `"<source>:<id>"`.
4. Start the live loop. On each tick, drop any ID already in the dedupe set; emit the rest with `"backfill": false`.

Prevents the boundary race where a session created milliseconds before the `--since` cutoff appears in both backfill and the first live tick.

The dedupe set is per-source — `local:ses_abc` and `cloud:ses_abc` are distinct keys.

## Polling and event detection

**Local source**:
- Primary: fsnotify on `~/Library/Application Support/disbug/local-sessions/sessions/`. New subdir → new session, query localstore by ID.
- Fallback: poll the `sessions` table on `--poll-interval` (default `2s`) if fsnotify fails (e.g. NFS mount).

**Cloud source**:
- Poll the sessions API on `--poll-interval` (default `30s`). Query is "all sessions created since last-seen timestamp, sorted ascending."
- Server-side long-poll or SSE can be added later without changing the JSONL contract.

`--poll-interval` is shared across sources for simplicity in v1. Split into `--local-poll` / `--cloud-poll` only if real usage demands it.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | Clean shutdown (SIGINT / SIGTERM). |
| 1 | Unexpected runtime error (localstore corrupt, network gone permanently, etc.). |
| 2 | `--cloud-only` set but not signed in. |
| 3 | Invalid flag value (`--since=invalid`, unknown `--status`, etc.). |

## Signals

- `SIGINT` / `SIGTERM`: graceful shutdown — flush stdout, kill in-flight `--exec`, exit 0.
- `SIGHUP`: reload auth token so a fresh `disbug login` in another shell takes effect without restart.

## Out of scope (v1)

- `--notify` flag. Notification is the consumer's job (Claude teammate via `Monitor` + `SendMessage`, or `--exec` script).
- `d` / `w` units in `--since`. `s` / `m` / `h` only.
- `session.updated` / `pin.added` event types. v1 is "new sessions only."
- Server-side streaming. Client-side polling is acceptable until token volume forces SSE.
- Persistent dedupe across watcher restarts. In-memory only — restart may briefly re-emit very recent items. Add only if real-world use shows it matters.

## Implementation order

1. Add `--since` parser (`s`/`m`/`h`, sanity cap `8760h`) to `disbug sessions` and `disbug local-sessions list`. Shared helper in `internal/timefmt` or similar.
2. Add `disbug watch` skeleton: flag parsing, `--local-only` path with fsnotify + sqlite tail, JSONL output. End-to-end on local source first.
3. Add cloud source + autodetection (signed-in check via existing `internal/auth`).
4. Add `--exec` hook with env-var contract.
5. (Later) server-side long-poll or SSE if polling tax becomes painful.

## Claude Code integration recipe (reference, not part of the CLI)

For documentation in `cli/docs/integrations/claude-code.md` once `watch` ships. Pseudocode for what a Claude teammate would do:

```
Monitor({
  command: "disbug watch --since=2h --format=jsonl",
  description: "new disbug sessions",
  persistent: true,
})

// Each stdout line wakes the teammate as a notification.
// On wake:
parse JSON
if event.backfill: skip
else:
  SendMessage({
    to: "lead",
    message: "New bug ${event.session.ref}: ${event.session.pins[0]?.feedback ?? '(no pins)'}",
  })
```

The CLI doesn't reference any of this. It's purely a consumer recipe.
