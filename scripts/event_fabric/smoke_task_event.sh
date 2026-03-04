#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

echo "[WARN] scripts/event_fabric/smoke_task_event.sh 已废弃，请改用 scripts/event_fabric/integration_playbook.sh"
exec "${SCRIPT_DIR}/integration_playbook.sh" "$@"
