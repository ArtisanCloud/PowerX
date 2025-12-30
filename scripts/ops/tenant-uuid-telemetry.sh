#!/usr/bin/env bash
set -euo pipefail

PROM_URL=${PROM_URL:-}
RANGE=${RANGE:-1h}
STEP=${STEP:-60}
OUTPUT=${OUTPUT:-}

usage() {
  cat <<'USAGE'
Usage: PROM_URL=https://prom.example.com scripts/ops/tenant-uuid-telemetry.sh [--range 1h] [--step 60] [--output report.md]

必填环境变量：
  PROM_URL      Prometheus 基础 URL，例如 https://prom.example.com
可选环境变量：
  RANGE         查询时间范围，默认 1h
  STEP          查询步长（秒），默认 60
  OUTPUT        输出文件路径，默认 stdout

脚本会查询以下指标：
  - tenant_header_reject_total
  - tenant_uuid_only_request_total
  - tenant_uuid_schema_drift
USAGE
}

range_start() {
  date -u -d "-${RANGE}" +%s 2>/dev/null || date -u -v -"${RANGE}" +%s
}

if [[ $# -gt 0 ]]; then
  while [[ $# -gt 0 ]]; do
    case "$1" in
      -h|--help) usage; exit 0 ;;
      --range) RANGE=$2; shift 2 ;;
      --step) STEP=$2; shift 2 ;;
      --output) OUTPUT=$2; shift 2 ;;
      *) echo "unknown arg: $1" >&2; usage; exit 1 ;;
    esac
  done
fi

[[ -n "$PROM_URL" ]] || { echo "PROM_URL env required" >&2; exit 1; }

query() {
  local metric=$1
  curl -sfSL "${PROM_URL}/api/v1/query_range" \
    --get --data-urlencode "query=${metric}" \
    --data-urlencode "step=${STEP}" \
    --data-urlencode "start=$(range_start)" \
    --data-urlencode "end=$(date -u +%s)"
}

report=$(cat <<REPORT
# Tenant UUID Telemetry ($(date -u +%Y-%m-%dT%H:%MZ))

* 范围：${RANGE}
* 步长：${STEP}s
* Prometheus：${PROM_URL}

## tenant_header_reject_total
$(query 'sum(rate(tenant_header_reject_total[5m]))')

## tenant_uuid_only_request_total
$(query 'sum(rate(tenant_uuid_only_request_total[5m]))')

## tenant_uuid_schema_drift
$(query 'sum(tenant_uuid_schema_drift)')
REPORT
)

if [[ -n "$OUTPUT" ]]; then
  echo "$report" > "$OUTPUT"
  echo "Report written to $OUTPUT"
else
  echo "$report"
fi
