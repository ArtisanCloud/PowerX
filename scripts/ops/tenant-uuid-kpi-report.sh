#!/usr/bin/env bash
set -euo pipefail

# 确保访问本地 Prometheus 不经过代理
export no_proxy="localhost,127.0.0.1,::1"
export NO_PROXY="localhost,127.0.0.1,::1"

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
PROJECT_ROOT=$(cd -- "$SCRIPT_DIR/../.." && pwd)
DEFAULT_OUTPUT_DIR="$PROJECT_ROOT/reports/tenant-uuid-kpi"
PROM_URL=${PROM_URL:-http://localhost:9090}
PROM_BEARER=${PROM_BEARER:-}
PROM_USER=${PROM_USER:-}
PROM_PASSWORD=${PROM_PASSWORD:-}
RANGE=${RANGE:-24h}
RATE_WINDOW=${RATE_WINDOW:-5m}
OUTPUT=${OUTPUT:-"$DEFAULT_OUTPUT_DIR/tenant-uuid-kpi-$(date -u +%Y-%m-%d).md"}

log() {
  printf '[tenant-kpi][pid=%d] %s\n' "$$" "$*" >&2
}

START_EPOCH=$(date +%s)
cleanup() {
  local duration=$(( $(date +%s) - START_EPOCH ))
  log "Finished in ${duration}s"
}
trap cleanup EXIT
usage() {
  cat <<'USAGE'
Usage: PROM_URL=https://prom.example.com scripts/ops/tenant-uuid-kpi-report.sh [options]

Options:
  --prom-url <url>        Prometheus 基础地址（也可通过 PROM_URL 环境变量设置）
  --range <duration>      increase 计算窗口，默认 24h
  --rate-window <dur>     rate 计算窗口，默认 5m
  --output <path>         输出 Markdown 路径（默认 reports/tenant-uuid-kpi/tenant-uuid-kpi-<date>.md，可指定 - 表示 stdout）
  -h, --help              显示帮助

可选环境变量：
  PROM_BEARER             Bearer Token（若 Prometheus 需要）
  PROM_USER / PROM_PASSWORD  基本认证账号/密码

脚本会输出包含以下指标的 Markdown 报告：
  - UUID-only 请求占比（基于 tenant_uuid_only_request_total / (uuid_only + rejects)）
  - Legacy header 拒绝次数（tenant_header_reject_total）
  - tenant_uuid_schema_drift / tenant_uuid_tables_without_uuid 观测值
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --prom-url)
      PROM_URL=$2
      shift 2
      ;;
    --range)
      RANGE=$2
      shift 2
      ;;
    --rate-window)
      RATE_WINDOW=$2
      shift 2
      ;;
    --output)
      OUTPUT=$2
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown option: $1" >&2
      usage
      exit 1
      ;;
  esac
done

[[ -n "$PROM_URL" ]] || { echo "[error] PROM_URL is required (env or --prom-url)" >&2; exit 1; }

AUTH_ARGS=()
if [[ -n "$PROM_BEARER" ]]; then
  AUTH_ARGS+=(-H "Authorization: Bearer $PROM_BEARER")
fi
if [[ -n "$PROM_USER" ]]; then
  AUTH_ARGS+=(-u "${PROM_USER}:${PROM_PASSWORD:-}")
fi

prom_value() {
  local expr="$1"
  local url="${PROM_URL%/}/api/v1/query"
  local resp
  local curl_cmd=(curl -sfSL)
  if ((${#AUTH_ARGS[@]})); then
    curl_cmd+=("${AUTH_ARGS[@]}")
  fi
  curl_cmd+=(--get "$url" --data-urlencode "query=${expr}" --data-urlencode "time=$(date -u +%s)")
  log "PromQL query: ${expr}"
  if ! resp=$("${curl_cmd[@]}"); then
    echo "[error] failed to query Prometheus for expr: ${expr}" >&2
    exit 1
  fi
  printf '%s' "$resp" | PROM_EXPR="$expr" python3 <<'PY'
import json, os, sys
expr = os.environ.get("PROM_EXPR", "")
try:
    data = json.load(sys.stdin)
except Exception:
    print("0")
    sys.exit(0)
if data.get("status") != "success":
    print("0")
    sys.exit(0)
result = data.get("data", {}).get("result", [])
if not result:
    print("0")
else:
    value = result[0].get("value", [None, "0"])[1]
    print(value if value is not None else "0")
PY
}

format_number() {
  local value=$1
  local decimals=${2:-2}
  python3 <<PY
import math
value = "$value"
decimals = $decimals
try:
    num = float(value)
except Exception:
    print(value or "0")
else:
    print(f"{num:.{decimals}f}")
PY
}

log "Fetching KPI metrics from Prometheus ${PROM_URL}"
uuid_rate=$(prom_value "sum(rate(tenant_uuid_only_request_total[${RATE_WINDOW}]))")
uuid_increase=$(prom_value "increase(tenant_uuid_only_request_total[${RANGE}])")
reject_rate=$(prom_value "sum(rate(tenant_header_reject_total[${RATE_WINDOW}]))")
reject_increase=$(prom_value "increase(tenant_header_reject_total[${RANGE}])")
schema_drift=$(prom_value "sum(tenant_uuid_schema_drift)")
tables_without_uuid=$(prom_value "sum(tenant_uuid_tables_without_uuid)")

uuid_rate_fmt=$(format_number "$uuid_rate" 4)
uuid_increase_fmt=$(format_number "$uuid_increase" 2)
reject_rate_fmt=$(format_number "$reject_rate" 4)
reject_increase_fmt=$(format_number "$reject_increase" 2)
schema_drift_fmt=$(format_number "$schema_drift" 0)
tables_without_uuid_fmt=$(format_number "$tables_without_uuid" 0)

uuid_ratio=$(python3 <<PY
import math
try:
    u = float("${uuid_rate}")
    r = float("${reject_rate}")
except Exception:
    print("N/A")
else:
    total = u + r
    if total <= 0:
        print("N/A")
    else:
        print(f"{(u / total) * 100:.2f}")
PY
)

uuid_ratio_display=$uuid_ratio
if [[ "$uuid_ratio" =~ ^[0-9.]+$ ]]; then
  uuid_ratio_display="${uuid_ratio}%"
fi

timestamp=$(date -u +%Y-%m-%dT%H:%M:%SZ)

read -r -d '' REPORT <<EOF || true
# Tenant UUID KPI 报告 (${timestamp})

- Prometheus：${PROM_URL}
- increase 统计范围：${RANGE}
- rate 统计窗口：${RATE_WINDOW}

## 指标速览
| 指标 | 当前值 | ${RANGE} 累计 | 目标 | 告警阈值 |
| --- | --- | --- | --- | --- |
| UUID-only 请求占比 | ${uuid_ratio_display} （有效 ${uuid_rate_fmt}/s） | 成功 ${uuid_increase_fmt} 次 | 100% | < 99.5% 连续 5 分钟 |
| Legacy header 拒绝 | ${reject_rate_fmt}/s | ${reject_increase_fmt} 次 | 0 | > 0 |
| Schema Drift 表计数 | ${schema_drift_fmt} | - | 0 | > 0 |
| 缺少 tenant_uuid 的表 | ${tables_without_uuid_fmt} | - | 0 | > 0 |

## PromQL 参考
- UUID-only rate：sum(rate(tenant_uuid_only_request_total[${RATE_WINDOW}]))
- Legacy header rejects：sum(rate(tenant_header_reject_total[${RATE_WINDOW}]))
- Schema drift：sum(tenant_uuid_schema_drift)
- Tables without tenant_uuid：sum(tenant_uuid_tables_without_uuid)
- Legacy header累计：increase(tenant_header_reject_total[${RANGE}])

> 生成脚本：`scripts/ops/tenant-uuid-kpi-report.sh`，支持 `PROM_URL`、`PROM_BEARER`、`PROM_USER` 等参数。若需周报可结合 `cron`，把输出保存到 `reports/tenant-uuid-kpi/` 并归档。
EOF

if [[ "$OUTPUT" == "-" ]]; then
  echo "$REPORT"
else
  mkdir -p "$(dirname "$OUTPUT")"
  printf '%s\n' "$REPORT" > "$OUTPUT"
  log "KPI report written to $OUTPUT"
fi
