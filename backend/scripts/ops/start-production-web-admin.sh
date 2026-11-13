#!/usr/bin/env bash
set -euo pipefail

# systemd 只执行本文件，避免 Unit → sh → awk 三层引号改变命令含义。
RUNTIME_ROOT="${POWERX_RUNTIME_ROOT:-/etc/powerx}"
LINKS_ROOT="${POWERX_LINKS_ROOT:-/opt/powerx}"
CFG="${POWERX_CONFIG:-${RUNTIME_ROOT}/config.yaml}"
[[ -f "${CFG}" ]] || CFG="${LINKS_ROOT}/backend/etc/config.yaml"
if [[ ! -r "${CFG}" ]]; then
  echo "POWERX_PROD_WEB_CONFIG_UNAVAILABLE" >&2
  exit 1
fi

backend_port="$(awk -F: '
  function indent(s) { match(s, /[^[:space:]]/); return RSTART > 0 ? RSTART - 1 : -1 }
  /^server:[[:space:]]*(#.*)?$/ { inside=1; level=-1; next }
  inside && /^[^[:space:]#]/ { inside=0 }
  inside && /^[[:space:]]*[A-Za-z_][A-Za-z_0-9]*:/ {
    current=indent($0)
    if (level<0) level=current
    if (current==level && $1 ~ /^[[:space:]]*port$/) { print int($2); exit }
  }
' "${CFG}")"
web_port="$(awk -F: '/^web_admin_port:[[:space:]]*/ { print int($2); exit }' "${CFG}")"
backend_port="${backend_port:-8080}"
web_port="${web_port:-3000}"
for port in "${backend_port}" "${web_port}"; do
  if [[ ! "${port}" =~ ^[0-9]+$ ]] || (( port < 1 || port > 65535 )); then
    echo "POWERX_PROD_WEB_PORT_INVALID" >&2
    exit 1
  fi
done
export POWERX_BACKEND="http://127.0.0.1:${backend_port}"
export NUXT_PUBLIC_POWERX_CORE_BASE="${POWERX_PUBLIC_BASE_URL:-${POWERX_GATEWAY_BASE_URL:-${POWERX_BACKEND}}}"
export NUXT_PUBLIC_WS_ORIGIN="${POWERX_PUBLIC_WS_ORIGIN:-${POWERX_GATEWAY_WS_BASE_URL:-ws://127.0.0.1:${backend_port}}}"
export NUXT_PUBLIC_WS_PATH=/api/ws
export PORT="${web_port}"
exec "${NODE_BIN:-/usr/bin/node}" "${LINKS_ROOT}/web-admin/.output/server/index.mjs"
