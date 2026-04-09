#!/usr/bin/env bash
set -euo pipefail

CMD="${1:-auto}" # auto | apply | clear | status
PROXY_URL="${DOCKER_PROXY_URL:-${2:-}}"
NO_PROXY_VALUE="${DOCKER_NO_PROXY:-localhost,127.0.0.1,::1}"
CONF_DIR="/etc/systemd/system/docker.service.d"
CONF_FILE="${CONF_DIR}/http-proxy.conf"

usage() {
  cat <<'EOF'
Usage:
  ./scripts/configure-docker-proxy.sh [auto|apply|clear|status] [proxy_url]

Examples:
  DOCKER_PROXY_URL=http://127.0.0.1:8890 ./scripts/configure-docker-proxy.sh auto
  ./scripts/configure-docker-proxy.sh apply http://127.0.0.1:8890
  ./scripts/configure-docker-proxy.sh status
  ./scripts/configure-docker-proxy.sh clear
EOF
}

print_status() {
  echo "[docker-proxy] docker service environment:"
  sudo systemctl show docker -p Environment --value || true
  if [[ -f "${CONF_FILE}" ]]; then
    echo "[docker-proxy] proxy file: ${CONF_FILE}"
    sudo cat "${CONF_FILE}"
  else
    echo "[docker-proxy] proxy file not found: ${CONF_FILE}"
  fi
}

apply_proxy() {
  if [[ -z "${PROXY_URL}" ]]; then
    echo "[docker-proxy] missing proxy url. set DOCKER_PROXY_URL or pass as arg" >&2
    exit 1
  fi
  echo "[docker-proxy] apply proxy: ${PROXY_URL}"
  sudo mkdir -p "${CONF_DIR}"
  cat <<EOF | sudo tee "${CONF_FILE}" >/dev/null
[Service]
Environment="HTTP_PROXY=${PROXY_URL}"
Environment="HTTPS_PROXY=${PROXY_URL}"
Environment="NO_PROXY=${NO_PROXY_VALUE}"
EOF
  sudo systemctl daemon-reload
  sudo systemctl restart docker
  print_status
}

clear_proxy() {
  echo "[docker-proxy] clear docker daemon proxy"
  sudo rm -f "${CONF_FILE}"
  sudo systemctl daemon-reload
  sudo systemctl restart docker
  print_status
}

check_registry_direct() {
  curl -fsS --connect-timeout 3 --max-time 5 https://registry-1.docker.io/v2/ >/dev/null
}

auto_mode() {
  echo "[docker-proxy] probe docker hub direct connectivity"
  if check_registry_direct; then
    echo "[docker-proxy] direct connect OK, keep current docker proxy config"
    print_status
    exit 0
  fi

  echo "[docker-proxy] direct connect FAILED"
  if [[ -z "${PROXY_URL}" ]]; then
    echo "[docker-proxy] DOCKER_PROXY_URL not set, cannot auto-apply proxy" >&2
    exit 1
  fi

  apply_proxy
}

case "${CMD}" in
  auto)
    auto_mode
    ;;
  apply)
    apply_proxy
    ;;
  clear)
    clear_proxy
    ;;
  status)
    print_status
    ;;
  -h|--help|help)
    usage
    ;;
  *)
    echo "[docker-proxy] unknown command: ${CMD}" >&2
    usage
    exit 1
    ;;
esac
