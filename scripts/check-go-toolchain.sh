#!/usr/bin/env bash
# Fails unless the Go in use is the toolchain pinned in go.mod, so CI and
# release builds cannot silently fall back to an older, unpatched Go.
set -euo pipefail

cd "$(dirname "$0")/.."

want=$(awk '$1 == "toolchain" { print $2 }' go.mod | tr -d '\r')
if [ -z "$want" ]; then
  echo "error: go.mod has no toolchain line" >&2
  exit 1
fi

have=$(go env GOVERSION | tr -d '\r')
if [ "$have" != "$want" ]; then
  echo "error: building with $have, but go.mod pins $want (check GOTOOLCHAIN and the setup-go step)" >&2
  exit 1
fi

echo "Go toolchain $have matches go.mod"
