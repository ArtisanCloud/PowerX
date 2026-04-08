#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<USAGE
Usage:
  $0 <target_tag_or_version> [--with-runner] [--with-setup-trace|--without-setup-trace] [--with-setup-reentry|--without-setup-reentry] [--timeout-sec N]

Description:
  Switch /opt/powerx symlinks to /opt/powerx/releases/<target_tag_or_version>,
  restart systemd services, and run backend health check.
  If health check fails, auto rollback to previous symlink targets.
  Production should use immutable git tag names (e.g. v2.0.2).

Options:
  --with-runner            Also switch/restart powerx-runner.
  --with-setup-trace       Enable setup/status trace log on powerx-backend.
  --without-setup-trace    Disable setup/status trace log on powerx-backend.
  --with-setup-reentry     Temporarily allow setup write APIs on installed instance.
  --without-setup-reentry  Disable setup reentry (recommended default).
  --timeout-sec N          Health check timeout seconds (default: 90).

Environment overrides:
  POWERX_RELEASES_ROOT     Default: /opt/powerx/releases
  POWERX_LINKS_ROOT        Default: /opt/powerx
  POWERX_RUNTIME_ROOT      Default: /etc/powerx
  POWERX_HEALTH_URL        Default: http://127.0.0.1:8080/api/v1/health
  POWERX_HEALTH_EXPECT     Default: 200
  POWERX_SERVICE_USER      Default: current login user (sudo user), fallback: powerx
  POWERX_SERVICE_GROUP     Default: primary group of POWERX_SERVICE_USER
USAGE
}

if [[ $# -lt 1 ]]; then
  usage >&2
  exit 1
fi

TARGET_REF="$1"
shift

WITH_RUNNER=0
WITH_SETUP_TRACE=0
WITHOUT_SETUP_TRACE=0
WITH_SETUP_REENTRY=0
WITHOUT_SETUP_REENTRY=0
TIMEOUT_SEC=90
while [[ $# -gt 0 ]]; do
  case "$1" in
    --with-runner)
      WITH_RUNNER=1
      shift
      ;;
    --with-setup-trace)
      WITH_SETUP_TRACE=1
      shift
      ;;
    --without-setup-trace)
      WITHOUT_SETUP_TRACE=1
      shift
      ;;
    --with-setup-reentry)
      WITH_SETUP_REENTRY=1
      shift
      ;;
    --without-setup-reentry)
      WITHOUT_SETUP_REENTRY=1
      shift
      ;;
    --timeout-sec)
      TIMEOUT_SEC="${2:-}"
      if [[ -z "$TIMEOUT_SEC" ]]; then
        echo "[switch-release] --timeout-sec requires a value" >&2
        exit 1
      fi
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "[switch-release] unknown arg: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

if [[ "$WITH_SETUP_TRACE" == "1" && "$WITHOUT_SETUP_TRACE" == "1" ]]; then
  echo "[switch-release] --with-setup-trace and --without-setup-trace cannot be used together" >&2
  exit 1
fi
if [[ "$WITH_SETUP_REENTRY" == "1" && "$WITHOUT_SETUP_REENTRY" == "1" ]]; then
  echo "[switch-release] --with-setup-reentry and --without-setup-reentry cannot be used together" >&2
  exit 1
fi

RELEASES_ROOT="${POWERX_RELEASES_ROOT:-/opt/powerx/releases}"
LINKS_ROOT="${POWERX_LINKS_ROOT:-/opt/powerx}"
RUNTIME_ROOT="${POWERX_RUNTIME_ROOT:-/etc/powerx}"
HEALTH_URL="${POWERX_HEALTH_URL:-http://127.0.0.1:8080/api/v1/health}"
HEALTH_EXPECT="${POWERX_HEALTH_EXPECT:-200}"
SERVICE_USER="${POWERX_SERVICE_USER:-${SUDO_USER:-powerx}}"
SERVICE_GROUP="${POWERX_SERVICE_GROUP:-}"
RUNTIME_CONFIG_PATH="${RUNTIME_ROOT}/config.yaml"
RUNTIME_SETUP_DRAFT_PATH="${RUNTIME_ROOT}/setup.wizard.config.json"

TARGET_ROOT="${RELEASES_ROOT}/${TARGET_REF}"
TARGET_BACKEND="${TARGET_ROOT}/backend"
TARGET_WEB_ADMIN="${TARGET_ROOT}/web-admin"
TARGET_RUNNER="${TARGET_ROOT}/runner"
TARGET_SYSTEMD="${TARGET_ROOT}/systemd"

LINK_BACKEND="${LINKS_ROOT}/backend"
LINK_WEB_ADMIN="${LINKS_ROOT}/web-admin"
LINK_RUNNER="${LINKS_ROOT}/runner"

if [[ "${EUID}" -ne 0 ]]; then
  echo "[switch-release] this script must run as root (use sudo)" >&2
  exit 1
fi

if [[ ! -d "$TARGET_BACKEND" ]]; then
  echo "[switch-release] missing target backend dir: $TARGET_BACKEND" >&2
  exit 1
fi
if [[ ! -d "$TARGET_WEB_ADMIN" ]]; then
  echo "[switch-release] missing target web-admin dir: $TARGET_WEB_ADMIN" >&2
  exit 1
fi
if [[ "$WITH_RUNNER" == "1" && ! -d "$TARGET_RUNNER" ]]; then
  echo "[switch-release] warning: target runner dir missing: $TARGET_RUNNER" >&2
  echo "[switch-release] warning: continue in noop-runner mode (service will be skipped by condition)" >&2
fi
if [[ "$WITH_RUNNER" == "1" && ! -f "$TARGET_RUNNER/dist/main.js" ]]; then
  echo "[switch-release] warning: runner artifact missing: $TARGET_RUNNER/dist/main.js" >&2
  echo "[switch-release] warning: continue in noop-runner mode (service will be skipped by condition)" >&2
fi

PREV_BACKEND=""
PREV_WEB_ADMIN=""
PREV_RUNNER=""

if [[ -L "$LINK_BACKEND" ]]; then
  PREV_BACKEND="$(readlink "$LINK_BACKEND")"
fi
if [[ -L "$LINK_WEB_ADMIN" ]]; then
  PREV_WEB_ADMIN="$(readlink "$LINK_WEB_ADMIN")"
fi
if [[ "$WITH_RUNNER" == "1" && -L "$LINK_RUNNER" ]]; then
  PREV_RUNNER="$(readlink "$LINK_RUNNER")"
fi

echo "[switch-release] target ref: $TARGET_REF"
echo "[switch-release] releases root: $RELEASES_ROOT"
echo "[switch-release] links root: $LINKS_ROOT"
echo "[switch-release] service identity: ${SERVICE_USER}:${SERVICE_GROUP:-<auto>}"

ensure_service_identity() {
  if id -u "${SERVICE_USER}" >/dev/null 2>&1; then
    if [[ -z "${SERVICE_GROUP}" ]]; then
      SERVICE_GROUP="$(id -gn "${SERVICE_USER}")"
    fi
  else
    if [[ -z "${SERVICE_GROUP}" ]]; then
      SERVICE_GROUP="${SERVICE_USER}"
    fi
    if ! getent group "${SERVICE_GROUP}" >/dev/null; then
      echo "[switch-release] create group: ${SERVICE_GROUP}"
      groupadd --system "${SERVICE_GROUP}"
    fi
    echo "[switch-release] create user: ${SERVICE_USER}"
    useradd --system \
      --gid "${SERVICE_GROUP}" \
      --home "${LINKS_ROOT}" \
      --shell /usr/sbin/nologin \
      "${SERVICE_USER}"
  fi

  if ! getent group "${SERVICE_GROUP}" >/dev/null; then
    echo "[switch-release] missing group: ${SERVICE_GROUP}" >&2
    exit 1
  fi

  install -d -m 0755 "${LINKS_ROOT}" "${RELEASES_ROOT}"
  install -d -m 0755 "${TARGET_BACKEND}/logs" "${TARGET_BACKEND}/logs/audit"
  chown -R "${SERVICE_USER}:${SERVICE_GROUP}" "${TARGET_ROOT}"
  chown -h "${SERVICE_USER}:${SERVICE_GROUP}" "${LINK_BACKEND}" "${LINK_WEB_ADMIN}" 2>/dev/null || true
  if [[ "$WITH_RUNNER" == "1" ]]; then
    chown -h "${SERVICE_USER}:${SERVICE_GROUP}" "${LINK_RUNNER}" 2>/dev/null || true
    install -d -m 0755 "${RUNTIME_ROOT}"
    if [[ ! -f "${RUNTIME_ROOT}/powerx.env" ]]; then
      if [[ -f "${TARGET_ROOT}/systemd/powerx.env.example" ]]; then
        cp "${TARGET_ROOT}/systemd/powerx.env.example" "${RUNTIME_ROOT}/powerx.env"
      else
        touch "${RUNTIME_ROOT}/powerx.env"
      fi
    fi
    chown root:root "${RUNTIME_ROOT}/powerx.env"
    chmod 0644 "${RUNTIME_ROOT}/powerx.env"
  fi
  ensure_node_bin_env
}

apply_service_user_override() {
  local unit="$1"
  local dir="/etc/systemd/system/${unit}.service.d"
  install -d -m 0755 "$dir"
  cat > "${dir}/zz-runtime-user.conf" <<EOF
[Service]
User=${SERVICE_USER}
Group=${SERVICE_GROUP}
EOF
}

apply_setup_trace_override() {
  local dir="/etc/systemd/system/powerx-backend.service.d"
  local file="${dir}/90-setup-trace.conf"
  install -d -m 0755 "$dir"
  cat > "$file" <<EOF
[Service]
Environment=POWERX_SETUP_STATUS_TRACE=1
EOF
  echo "[switch-release] setup trace enabled: ${file}"
}

remove_setup_trace_override() {
  local file="/etc/systemd/system/powerx-backend.service.d/90-setup-trace.conf"
  if [[ -f "$file" ]]; then
    rm -f "$file"
    echo "[switch-release] setup trace disabled: ${file}"
  fi
}

assert_effective_service_user() {
  local unit="$1"
  local expected_user="$2"
  local expected_group="$3"
  local unit_base="${unit%.service}"
  local actual_user=""
  local actual_group=""
  actual_user="$(systemctl show "${unit}" -p User --value | xargs)"
  actual_group="$(systemctl show "${unit}" -p Group --value | xargs)"
  if [[ "$actual_user" != "$expected_user" || "$actual_group" != "$expected_group" ]]; then
    echo "[switch-release] error: ${unit} effective identity mismatch (expected ${expected_user}:${expected_group}, got ${actual_user}:${actual_group})" >&2
    echo "[switch-release] hint: check /etc/systemd/system/${unit_base}.service.d/*.conf for conflicting User/Group overrides" >&2
    exit 1
  fi
}

detect_node_bin() {
  local candidate=""
  local home_dir=""
  local env_file="${RUNTIME_ROOT}/powerx.env"

  # 优先使用用户显式指定的 NODE_BIN（若可执行）
  if [[ -f "$env_file" ]]; then
    candidate="$(awk -F= '/^NODE_BIN=/{print substr($0,10); exit}' "$env_file" | tr -d '"' | xargs)"
    if [[ -n "$candidate" ]] && [[ -x "$candidate" ]] && sudo -u "${SERVICE_USER}" test -x "$candidate" 2>/dev/null; then
      printf "%s" "$candidate"
      return 0
    fi
  fi

  # 优先系统级路径，避免依赖 sudo 用户自己的 nvm 目录权限。
  for candidate in /usr/bin/node /usr/local/bin/node; do
    if [[ -x "$candidate" ]] && sudo -u "${SERVICE_USER}" test -x "$candidate" 2>/dev/null; then
      printf "%s" "$candidate"
      return 0
    fi
  done

  # 其次尝试当前 PATH（可能是系统级，也可能是可访问的自定义路径）
  if command -v node >/dev/null 2>&1; then
    candidate="$(command -v node)"
    if [[ -x "$candidate" ]] && sudo -u "${SERVICE_USER}" test -x "$candidate" 2>/dev/null; then
      printf "%s" "$candidate"
      return 0
    fi
  fi

  # 再尝试 sudo 原用户 PATH（常见为 nvm），但仍要求 powerx 可执行。
  if [[ -n "${SUDO_USER:-}" ]]; then
    candidate="$(sudo -u "${SUDO_USER}" bash -lc 'command -v node || true' 2>/dev/null || true)"
    if [[ -n "$candidate" ]] && [[ -x "$candidate" ]] && sudo -u "${SERVICE_USER}" test -x "$candidate" 2>/dev/null; then
      printf "%s" "$candidate"
      return 0
    fi
  fi

  # 兜底：显式扫描常见 nvm 安装路径（非交互 shell 下 command -v 可能拿不到）。
  for u in "${SERVICE_USER}" "${SUDO_USER:-}"; do
    [[ -z "$u" ]] && continue
    home_dir="$(getent passwd "$u" 2>/dev/null | cut -d: -f6 || true)"
    [[ -z "$home_dir" ]] && continue
    for candidate in "${home_dir}"/.nvm/versions/node/*/bin/node; do
      if [[ -x "$candidate" ]] && sudo -u "${SERVICE_USER}" test -x "$candidate" 2>/dev/null; then
        printf "%s" "$candidate"
        return 0
      fi
    done
  done

  return 1
}

ensure_node_bin_env() {
  local env_file="${RUNTIME_ROOT}/powerx.env"
  local node_bin
  install -d -m 0755 "${RUNTIME_ROOT}"
  if [[ ! -f "$env_file" ]]; then
    if [[ -f "${TARGET_ROOT}/systemd/powerx.env.example" ]]; then
      cp "${TARGET_ROOT}/systemd/powerx.env.example" "$env_file"
    else
      touch "$env_file"
    fi
    chown root:root "$env_file"
    chmod 0644 "$env_file"
  fi

  if ! node_bin="$(detect_node_bin)"; then
    cat >&2 <<EOF
[switch-release] error: no executable node found for user '${SERVICE_USER}'.
[switch-release] required by: powerx-web-admin.service / powerx-runner.service
[switch-release] options:
  1) install system node and set NODE_BIN=/usr/bin/node
  2) use custom node path and ensure '${SERVICE_USER}' has execute permission on full path (ACL/chmod)
EOF
    exit 1
  fi

  if grep -q '^NODE_BIN=' "$env_file"; then
    sed -i "s|^NODE_BIN=.*$|NODE_BIN=${node_bin}|" "$env_file"
  else
    printf '\nNODE_BIN=%s\n' "$node_bin" >> "$env_file"
  fi

  # 固化运行时根路径变量，供 backend/setup/service 统一读取。
  if grep -q '^POWERX_RUNTIME_ROOT=' "$env_file"; then
    sed -i "s|^POWERX_RUNTIME_ROOT=.*$|POWERX_RUNTIME_ROOT=${RUNTIME_ROOT}|" "$env_file"
  else
    printf 'POWERX_RUNTIME_ROOT=%s\n' "${RUNTIME_ROOT}" >> "$env_file"
  fi
  if grep -q '^POWERX_LINKS_ROOT=' "$env_file"; then
    sed -i "s|^POWERX_LINKS_ROOT=.*$|POWERX_LINKS_ROOT=${LINKS_ROOT}|" "$env_file"
  else
    printf 'POWERX_LINKS_ROOT=%s\n' "${LINKS_ROOT}" >> "$env_file"
  fi
  if grep -q '^POWERX_RELEASES_ROOT=' "$env_file"; then
    sed -i "s|^POWERX_RELEASES_ROOT=.*$|POWERX_RELEASES_ROOT=${RELEASES_ROOT}|" "$env_file"
  else
    printf 'POWERX_RELEASES_ROOT=%s\n' "${RELEASES_ROOT}" >> "$env_file"
  fi
  if grep -q '^POWERX_CONFIG=' "$env_file"; then
    sed -i "s|^POWERX_CONFIG=.*$|POWERX_CONFIG=${RUNTIME_CONFIG_PATH}|" "$env_file"
  else
    printf 'POWERX_CONFIG=%s\n' "${RUNTIME_CONFIG_PATH}" >> "$env_file"
  fi

  echo "[switch-release] using NODE_BIN=${node_bin}"
}

set_setup_reentry_env() {
  local env_file="${RUNTIME_ROOT}/powerx.env"
  if [[ ! -f "$env_file" ]]; then
    touch "$env_file"
    chown root:root "$env_file"
    chmod 0644 "$env_file"
  fi

  if [[ "$WITH_SETUP_REENTRY" == "1" ]]; then
    if grep -q '^POWERX_ALLOW_SETUP_REENTRY=' "$env_file"; then
      sed -i "s|^POWERX_ALLOW_SETUP_REENTRY=.*$|POWERX_ALLOW_SETUP_REENTRY=true|" "$env_file"
    else
      printf '\nPOWERX_ALLOW_SETUP_REENTRY=true\n' >> "$env_file"
    fi
    echo "[switch-release] setup reentry enabled via ${env_file}"
  elif [[ "$WITHOUT_SETUP_REENTRY" == "1" ]]; then
    sed -i '/^POWERX_ALLOW_SETUP_REENTRY=/d' "$env_file"
    echo "[switch-release] setup reentry disabled via ${env_file}"
  fi
}

ensure_runtime_config_external() {
  local source_cfg=""
  local source_draft=""

  install -d -m 0755 "${RUNTIME_ROOT}"

  if [[ ! -f "${RUNTIME_CONFIG_PATH}" ]]; then
    if [[ -n "${PREV_BACKEND}" ]] && [[ -f "${PREV_BACKEND}/etc/config.yaml" ]]; then
      source_cfg="${PREV_BACKEND}/etc/config.yaml"
    elif [[ -f "${TARGET_BACKEND}/etc/config.yaml" ]]; then
      source_cfg="${TARGET_BACKEND}/etc/config.yaml"
    fi

    if [[ -n "${source_cfg}" ]]; then
      cp "${source_cfg}" "${RUNTIME_CONFIG_PATH}"
      chown "${SERVICE_USER}:${SERVICE_GROUP}" "${RUNTIME_CONFIG_PATH}"
      chmod 0644 "${RUNTIME_CONFIG_PATH}"
      echo "[switch-release] runtime config initialized: ${RUNTIME_CONFIG_PATH} <= ${source_cfg}"
    else
      echo "[switch-release] warning: runtime config source not found, keep release-local config fallback" >&2
    fi
  else
    echo "[switch-release] runtime config preserved: ${RUNTIME_CONFIG_PATH}"
  fi

  # setup/provision/complete 需要写 runtime config，确保运行用户可写。
  if [[ -f "${RUNTIME_CONFIG_PATH}" ]]; then
    chown "${SERVICE_USER}:${SERVICE_GROUP}" "${RUNTIME_CONFIG_PATH}"
    chmod 0644 "${RUNTIME_CONFIG_PATH}"
  fi

  if [[ ! -f "${RUNTIME_SETUP_DRAFT_PATH}" ]]; then
    if [[ -n "${PREV_BACKEND}" ]] && [[ -f "${PREV_BACKEND}/etc/setup.wizard.config.json" ]]; then
      source_draft="${PREV_BACKEND}/etc/setup.wizard.config.json"
    elif [[ -f "${TARGET_BACKEND}/etc/setup.wizard.config.json" ]]; then
      source_draft="${TARGET_BACKEND}/etc/setup.wizard.config.json"
    fi
    if [[ -n "${source_draft}" ]]; then
      cp "${source_draft}" "${RUNTIME_SETUP_DRAFT_PATH}"
      chown "${SERVICE_USER}:${SERVICE_GROUP}" "${RUNTIME_SETUP_DRAFT_PATH}"
      chmod 0644 "${RUNTIME_SETUP_DRAFT_PATH}"
      echo "[switch-release] setup draft initialized: ${RUNTIME_SETUP_DRAFT_PATH} <= ${source_draft}"
    else
      printf '{}\n' > "${RUNTIME_SETUP_DRAFT_PATH}"
      chown "${SERVICE_USER}:${SERVICE_GROUP}" "${RUNTIME_SETUP_DRAFT_PATH}"
      chmod 0644 "${RUNTIME_SETUP_DRAFT_PATH}"
      echo "[switch-release] setup draft initialized: ${RUNTIME_SETUP_DRAFT_PATH}"
    fi
  fi

  if [[ -f "${RUNTIME_SETUP_DRAFT_PATH}" ]]; then
    chown "${SERVICE_USER}:${SERVICE_GROUP}" "${RUNTIME_SETUP_DRAFT_PATH}"
    chmod 0644 "${RUNTIME_SETUP_DRAFT_PATH}"
  fi
}

rollback() {
  echo "[switch-release] rollback start"
  if [[ -n "$PREV_BACKEND" ]]; then
    ln -sfn "$PREV_BACKEND" "$LINK_BACKEND"
  fi
  if [[ -n "$PREV_WEB_ADMIN" ]]; then
    ln -sfn "$PREV_WEB_ADMIN" "$LINK_WEB_ADMIN"
  fi
  if [[ "$WITH_RUNNER" == "1" && -n "$PREV_RUNNER" ]]; then
    ln -sfn "$PREV_RUNNER" "$LINK_RUNNER"
  fi

  systemctl daemon-reload
  systemctl restart powerx-backend powerx-web-admin
  if [[ "$WITH_RUNNER" == "1" ]]; then
    systemctl restart powerx-runner
  fi
  echo "[switch-release] rollback done"
}

trap 'echo "[switch-release] error occurred" >&2; rollback' ERR

ln -sfn "$TARGET_BACKEND" "$LINK_BACKEND"
ln -sfn "$TARGET_WEB_ADMIN" "$LINK_WEB_ADMIN"
if [[ "$WITH_RUNNER" == "1" ]]; then
  ln -sfn "$TARGET_RUNNER" "$LINK_RUNNER"
fi

ensure_service_identity
ensure_runtime_config_external
set_setup_reentry_env

if [[ -d "$TARGET_SYSTEMD" ]]; then
  cp "$TARGET_SYSTEMD"/*.service /etc/systemd/system/
fi
apply_service_user_override "powerx-backend"
apply_service_user_override "powerx-web-admin"
if [[ "$WITH_RUNNER" == "1" ]]; then
  apply_service_user_override "powerx-runner"
fi
if [[ "$WITH_SETUP_TRACE" == "1" ]]; then
  apply_setup_trace_override
fi
if [[ "$WITHOUT_SETUP_TRACE" == "1" ]]; then
  remove_setup_trace_override
fi

systemctl daemon-reload
assert_effective_service_user "powerx-backend.service" "${SERVICE_USER}" "${SERVICE_GROUP}"
assert_effective_service_user "powerx-web-admin.service" "${SERVICE_USER}" "${SERVICE_GROUP}"
if [[ "$WITH_RUNNER" == "1" ]]; then
  assert_effective_service_user "powerx-runner.service" "${SERVICE_USER}" "${SERVICE_GROUP}"
fi
if [[ "$WITH_RUNNER" == "1" ]]; then
  systemctl enable powerx-backend powerx-web-admin powerx-runner
else
  systemctl enable powerx-backend powerx-web-admin
fi
systemctl restart powerx-backend powerx-web-admin
if [[ "$WITH_RUNNER" == "1" ]]; then
  systemctl restart powerx-runner
fi

START_TS="$(date +%s)"
while true; do
  CODE="$(curl -sS -o /tmp/powerx-switch-health.json -w '%{http_code}' "$HEALTH_URL" || true)"
  if [[ "$CODE" == "$HEALTH_EXPECT" ]]; then
    break
  fi
  NOW_TS="$(date +%s)"
  if (( NOW_TS - START_TS >= TIMEOUT_SEC )); then
    echo "[switch-release] health check timeout, url=$HEALTH_URL code=$CODE expect=$HEALTH_EXPECT" >&2
    exit 1
  fi
  sleep 2
done

trap - ERR

echo "[switch-release] success: switched to ${TARGET_REF}"
systemctl --no-pager --full status powerx-backend powerx-web-admin | sed -n '1,40p'
if [[ "$WITH_RUNNER" == "1" ]]; then
  systemctl --no-pager --full status powerx-runner | sed -n '1,20p'
fi
