#!/usr/bin/env bash
set -euo pipefail
shopt -s nocasematch

TARGET=${GIT_DIFF_TARGET:---cached}
ALLOW_REGEX=${TENANT_ID_ALLOW_REGEX:-"^(scripts/migrations/tenant-uuid/|tmp/tenant-id-report\.md)"}

if [[ "$TARGET" == "--cached" ]]; then
  DIFF_CMD=(git diff --cached --unified=0)
else
  DIFF_CMD=(git diff --unified=0 "$TARGET")
fi

diff_output="$(${DIFF_CMD[@]})"
if [[ -z "$diff_output" ]]; then
  echo "[tenant-id-check] no diff to inspect"
  exit 0
fi

declare -a patterns=(
  "tenant_id[[:space:]]+IS[[:space:]]+NOT[[:space:]]+DISTINCT[[:space:]]+FROM"
  "tenant_id[[:space:]]+IN[[:space:]]*\\("
  "tenant_id[[:space:]]*(<>|!=)"
  "tenant[_-]?id"
  "tenantresolver"
  "tidalias"
)
declare -a reasons=(
  "SQL uses tenant_id IS NOT DISTINCT FROM"
  "SQL uses tenant_id IN (...)"
  "SQL uses tenant_id comparison operator"
  "legacy tenant_id usage"
  "tenantresolver stub detected"
  "TidAlias stub detected"
)

violations=()
current_file=""
while IFS= read -r line; do
  case "$line" in
    "+++ b/*")
      current_file=${line#+++ b/}
      ;;
    +*)
      if [[ -z "$current_file" ]]; then
        continue
      fi
      if [[ "$current_file" =~ $ALLOW_REGEX ]]; then
        continue
      fi
      for idx in "${!patterns[@]}"; do
        if [[ "$line" =~ ${patterns[$idx]} ]]; then
          violations+=("${current_file}:${line} <-- ${reasons[$idx]}")
          break
        fi
      done
      ;;
  esac
done <<< "$diff_output"

if [[ ${#violations[@]} -eq 0 ]]; then
  echo "[tenant-id-check] passed"
  exit 0
fi

echo "[tenant-id-check] detected forbidden tenant stubs/fields:" >&2
printf '  %s\n' "${violations[@]}" >&2
exit 1
