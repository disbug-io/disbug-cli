#!/usr/bin/env bash
set -euo pipefail

SCHEMA_URL="${DISBUG_SCHEMA_URL:-https://disbug.io/api/schema/}"
TMP=$(mktemp /tmp/disbug-schema-XXXXXX.yaml)
trap "rm -f $TMP" EXIT
curl -fsSL "$SCHEMA_URL" -o "$TMP"
.tools/oapi-codegen --config internal/client/oapi-codegen.yaml "$TMP"
gofmt -w internal/client/generated.go
