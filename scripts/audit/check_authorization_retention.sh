#!/usr/bin/env bash

set -euo pipefail

if ! command -v clickhouse-client >/dev/null 2>&1; then
  echo "[ERROR] clickhouse-client 未安装，无法执行检查" >&2
  exit 1
fi

TENANT_ID="${1:-}"
if [[ -z "$TENANT_ID" ]]; then
  echo "用法: $0 <tenant_uuid> [--days <N>]" >&2
  exit 1
fi

DAYS=1095
if [[ "${2:-}" == "--days" ]]; then
  DAYS="${3:-1095}"
fi

THRESHOLD=$(python3 - <<'PY'
import sys
from datetime import datetime, timedelta, timezone
print((datetime.now(timezone.utc)-timedelta(days=int(sys.argv[1]))).strftime('%Y-%m-%dT%H:%M:%SZ'))
PY
"${DAYS}")

HOST="${CLICKHOUSE_HOST:-localhost}"
PORT="${CLICKHOUSE_PORT:-9000}"
USER="${CLICKHOUSE_USER:-default}"
PASS="${CLICKHOUSE_PASSWORD:-}"
QUERY="SELECT min(occurred_at) AS min_ts, max(occurred_at) AS max_ts, sum(occurred_at < toDateTime('${THRESHOLD}')) AS legacy_rows FROM audit.authorization WHERE tenant_id = '${TENANT_ID}'"

CMD=(clickhouse-client --host "$HOST" --port "$PORT" --query "$QUERY")
if [[ -n "$PASS" ]]; then
  CMD+=(--password "$PASS" --user "$USER")
else
  CMD+=(--user "$USER")
fi

echo "[INFO] 审计留存阈值: ${THRESHOLD}"
"${CMD[@]}"
