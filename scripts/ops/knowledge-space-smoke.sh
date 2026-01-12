#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
REPORT_PATH="${1:-$ROOT_DIR/docs/releases/knowledge_space_smoke_report.md}"

mkdir -p "$(dirname "$REPORT_PATH")"

echo "== Knowledge Space Smoke (T058) =="
echo "- report: $REPORT_PATH"
echo "- repo:   $ROOT_DIR"

timestamp="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
branch="$(git -C "$ROOT_DIR" rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")"
commit="$(git -C "$ROOT_DIR" rev-parse --short HEAD 2>/dev/null || echo "unknown")"

run_step() {
  local title="$1"
  shift
  echo ""
  echo "---- $title ----"
  if "$@"; then
    echo "✅ $title: PASS"
    return 0
  else
    echo "❌ $title: FAIL"
    return 1
  fi
}

cat >"$REPORT_PATH" <<EOF
# Knowledge Space 全链路冒烟报告（T058）

- 执行时间（UTC）：$timestamp
- 分支：$branch
- 提交：$commit

> 说明：此报告由 \`scripts/ops/knowledge-space-smoke.sh\` 生成（会覆盖写入）。

## 自动化测试

EOF

FAILS=0

(
  cd "$ROOT_DIR/backend"
  mkdir -p "$ROOT_DIR/tmp/gocache" "$ROOT_DIR/tmp/gomodcache" || true
  export GOCACHE="$ROOT_DIR/tmp/gocache"
  export GOMODCACHE="$ROOT_DIR/tmp/gomodcache"
) >/dev/null 2>&1 || true

if run_step "Backend contract tests" bash -lc "cd \"$ROOT_DIR/backend\" && go test ./tests/contract/knowledge_space/..."; then
  echo "- Backend Contract：PASS" >>"$REPORT_PATH"
else
  echo "- Backend Contract：FAIL" >>"$REPORT_PATH"
  FAILS=$((FAILS+1))
fi

if run_step "Backend integration tests" bash -lc "cd \"$ROOT_DIR/backend\" && go test ./tests/integration/knowledge_space/..."; then
  echo "- Backend Integration：PASS" >>"$REPORT_PATH"
else
  echo "- Backend Integration：FAIL" >>"$REPORT_PATH"
  FAILS=$((FAILS+1))
fi

if run_step "Web unit tests" bash -lc "cd \"$ROOT_DIR/web-admin\" && npm run test:unit -- tests/unit/knowledge-spaces/ingestion.spec.ts"; then
  echo "- Web Unit：PASS" >>"$REPORT_PATH"
else
  echo "- Web Unit：FAIL" >>"$REPORT_PATH"
  FAILS=$((FAILS+1))
fi

if run_step "Web e2e tests (grep=knowledge-spaces)" bash -lc "cd \"$ROOT_DIR/web-admin\" && npm run test:e2e -- --grep \"knowledge-spaces\""; then
  echo "- Web E2E：PASS" >>"$REPORT_PATH"
else
  echo "- Web E2E：FAIL" >>"$REPORT_PATH"
  FAILS=$((FAILS+1))
fi

cat >>"$REPORT_PATH" <<'EOF'

## 报表校验（reports/_state）

执行：

```bash
node scripts/ops/knowledge-space-smoke-verify.mjs
```

结果：_PASS/FAIL_

## 手动链路确认（Quickstart 第 7 步）

请按 `specs/011-knowledge-space/quickstart.md` 执行，并在此记录 spaceId、策略 id、关键截图路径。

EOF

echo ""
if [[ "$FAILS" -eq 0 ]]; then
  echo "✅ smoke done: PASS"
  exit 0
fi
echo "❌ smoke done: FAILS=$FAILS (see report: $REPORT_PATH)"
exit 1

