#!/usr/bin/env bash
# 检查 tenant uuid 规范：禁止手动 `uuid.Parse`，并阻止新增 `tenant_id = ?` SQL。
set -euo pipefail

PROJECT_ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
TARGET_DIR=${TENANT_UUID_SCAN_DIR:-"$PROJECT_ROOT/backend"}
ALLOW_REGEX=${TENANT_UUID_CANONICAL_ALLOW_REGEX:-""}
declare -a PATTERNS=()
declare -a PATTERN_HINTS=()

add_rule() {
  PATTERNS+=("$1")
  PATTERN_HINTS+=("$2")
}

add_rule '(?i)uuid\.Parse\([^)]*tenant[_]?uuid' '检测到手动解析 tenant uuid，请改用 reqctx.RequireTenantUUID* 或 tenant scope helper。'
add_rule '(?i)uuid\.MustParse\([^)]*tenant[_]?uuid' '检测到手动解析 tenant uuid，请改用 reqctx.RequireTenantUUID* 或 tenant scope helper。'
add_rule '(?i)tenant[_]?id\s*=\s*\?' '检测到新的 `tenant_id = ?` SQL，请改为使用 tenant_uuid 条件或迁移后的仓储方法。'

if ! command -v rg >/dev/null 2>&1; then
  echo "[tenant-uuid-canonical] 需要安装 ripgrep (rg)" >&2
  exit 1
fi

cd "$PROJECT_ROOT"

violations=()
VIOLATION_HINT_SEP="__TENANT_UUID_HINT__"

record_violation() {
  local file="$1"
  local payload="$2"
  local hint="$3"
  local relative="${file#"$PROJECT_ROOT"/}"
  if [[ -n "$ALLOW_REGEX" && "$relative" =~ $ALLOW_REGEX ]]; then
    return 0
  fi
  violations+=("$relative:$payload${VIOLATION_HINT_SEP}${hint}")
}

scan_pattern() {
  local pattern="$1"
  local hint="$2"
  while IFS= read -r line; do
    [[ -z "$line" ]] && continue
    local file=${line%%:*}
    local payload=${line#*:}
    record_violation "$file" "$payload" "$hint"
  done < <(rg --no-heading --line-number --color=never --pcre2 -i "$pattern" "$TARGET_DIR" || true)
}

for idx in "${!PATTERNS[@]}"; do
  scan_pattern "${PATTERNS[$idx]}" "${PATTERN_HINTS[$idx]}"
done

if [[ ${#violations[@]} -eq 0 ]]; then
  echo "[tenant-uuid-canonical] passed"
  exit 0
fi

{
  echo "[tenant-uuid-canonical] 检测到以下违规项："
  for violation in "${violations[@]}"; do
    location=${violation%%${VIOLATION_HINT_SEP}*}
    hint=${violation#*${VIOLATION_HINT_SEP}}
    printf '  %s\n    ↳ %s\n' "$location" "$hint"
  done
} >&2

exit 1
