#!/usr/bin/env bash
# Prints one release's section of CHANGELOG.md, for use as its GitHub release
# notes. Fails if the section is missing or empty.
#
# Usage: scripts/release-notes.sh <tag or version> [changelog]
#   scripts/release-notes.sh v0.4.3   # prints the "## [0.4.3] - <date>" section
set -euo pipefail

if [ $# -lt 1 ] || [ $# -gt 2 ]; then
  echo "usage: $0 <tag or version> [changelog]" >&2
  exit 2
fi

version="${1#v}"
changelog="${2:-CHANGELOG.md}"

fail() {
  if [ "${GITHUB_ACTIONS:-}" = "true" ]; then
    echo "::error file=$changelog::$1" >&2
  else
    echo "error: $1" >&2
  fi
  exit 1
}

[ -f "$changelog" ] || fail "$changelog not found"

# The section runs from its "## [x.y.z]" heading to the next "## " heading.
# Leading and trailing blank lines are dropped.
notes=$(awk -v version="$version" '
  { sub(/\r$/, "") }
  /^## / {
    if (found) exit
    heading = $0
    if (sub(/^## \[/, "", heading) && sub(/\].*$/, "", heading) && heading == version) {
      found = 1
      next
    }
  }
  found { print }
' "$changelog" | sed -e '/[^[:space:]]/,$!d')

if [ -z "$(printf '%s' "$notes" | tr -d '[:space:]')" ]; then
  fail "$changelog has no notes for $version. Add a \"## [$version] - YYYY-MM-DD\" section (move the [Unreleased] entries under it) before tagging v$version."
fi

printf '%s\n' "$notes"
