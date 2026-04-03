#!/usr/bin/env bash
set -euo pipefail

MIGRATION_ID="${1:-0}"
SOURCE_ENV="${2:-unknown}"
TARGET_ENV="${3:-unknown}"

printf '[migration:%s] verify source=%s target=%s\n' "$MIGRATION_ID" "$SOURCE_ENV" "$TARGET_ENV"
exit 0
