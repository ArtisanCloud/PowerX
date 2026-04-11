#!/usr/bin/env bash
set -euo pipefail

# 触发单策略备份。
# 参数: <policy_id>
# 约定:
# - 成功返回 0，失败返回非 0。
# - 通过 stdout 输出结构化前缀 [backup-db] 便于日志检索。
POLICY_ID="${1:-}"
if [[ -z "${POLICY_ID}" ]]; then
  echo "usage: $0 <policy_id>" >&2
  exit 1
fi

echo "[backup-db] start policy=${POLICY_ID} at $(date -u +%FT%TZ)"
# placeholder: pg_dump / snapshot command
sleep 0.1
echo "[backup-db] done policy=${POLICY_ID}"
