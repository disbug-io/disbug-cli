#!/usr/bin/env bash
set -euo pipefail

BIN="${BIN:-./bin/disbug}"

if [[ -z "${DISBUG_API_URL:-}" ]]; then
  echo "Set DISBUG_API_URL to a Disbug instance" >&2
  exit 1
fi

if [[ -z "${DISBUG_LOGIN_TOKEN:-}" ]]; then
  echo "Set DISBUG_LOGIN_TOKEN to a known-good agent token" >&2
  exit 1
fi

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
export XDG_CONFIG_HOME="$TMP"
unset DISBUG_TOKEN

fail() {
  echo "FAIL: $1" >&2
  exit 1
}

run_step() {
  local step="$1"
  shift

  "$@" || fail "$step"
}

run_step "login" "$BIN" login --token-from-env --force --api-url "$DISBUG_API_URL"
run_step "whoami" "$BIN" whoami --pretty
run_step "doctor" "$BIN" doctor
run_step "sessions" "$BIN" sessions --limit 5 --pretty

SESSIONS_JSON="$TMP/sessions.json"
run_step "sessions --limit 1" "$BIN" sessions --limit 1 --pretty >"$SESSIONS_JSON"

SESSION_URL=$(
  python3 - "$SESSIONS_JSON" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    payload = json.load(handle)

results = payload.get("results") or []
if not results:
    sys.exit(2)

session_url = results[0].get("report_url")
if session_url in (None, ""):
    sys.exit(3)

print(session_url)
PY
) || {
  status=$?
  if [[ "$status" -eq 2 ]]; then
    fail "sessions returned no results"
  fi
  fail "extract first session URL"
}

SESSION_JSON="$TMP/session.json"
if ! "$BIN" session "$SESSION_URL" --pretty | tee "$SESSION_JSON"; then
  fail "session $SESSION_URL"
fi

PIN_NUMBER=$(
  python3 - "$SESSION_JSON" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    payload = json.load(handle)

pins = payload.get("pins") or []
if not pins:
    sys.exit(2)

pin_number = pins[0].get("number")
if pin_number in (None, ""):
    sys.exit(3)

print(pin_number)
PY
) || {
  status=$?
  if [[ "$status" -eq 2 ]]; then
    fail "session $SESSION_URL returned no pins"
  fi
  fail "extract first pin number"
}

PIN_URL="${SESSION_URL%/}/?pin=$PIN_NUMBER"
run_step "pin $PIN_URL" "$BIN" pin "$PIN_URL" --fields screenshot --pretty

echo "PASS"
