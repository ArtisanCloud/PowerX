#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<USAGE
Usage:
  $0 <target_tag_or_version> [--with-runner] [--with-setup-trace|--without-setup-trace] [--with-setup-reentry|--without-setup-reentry] [--timeout-sec N]

Description:
  Switch release symlinks to the target release,
  restart systemd services, and run backend health check.
  If health check fails, auto rollback to previous symlink targets.
  Production should use immutable git tag names (e.g. v2.0.2).

Options:
  --with-runner            Also switch/restart runner service.
  --with-setup-trace       Enable setup/status trace log on backend service.
  --without-setup-trace    Disable setup/status trace log on backend service.
  --with-setup-reentry     Temporarily allow setup write APIs on installed instance.
  --without-setup-reentry  Disable setup reentry (recommended default).
  --timeout-sec N          Health check timeout seconds (default: 90).

Environment overrides:
  POWERX_RELEASES_ROOT     Default: /opt/powerx/releases
  POWERX_LINKS_ROOT        Default: /opt/powerx
  POWERX_RUNTIME_ROOT      Default: /etc/powerx
  POWERX_STORAGE_ROOT      Default: /opt/powerx/storage
  POWERX_PLUGIN_RUNTIME_ROOT
                            Default: POWERX_LINKS_ROOT/plugins
  POWERX_HEALTH_URL        Default: http://127.0.0.1:8080/api/v1/health
  POWERX_HEALTH_EXPECT     Default: 200
  POWERX_SERVICE_USER      Default: current login user (sudo user), fallback: powerx
  POWERX_SERVICE_GROUP     Default: primary group of POWERX_SERVICE_USER
  POWERX_BACKEND_SERVICE   Default: powerx-backend
  POWERX_WEB_ADMIN_SERVICE Default: powerx-web-admin
  POWERX_RUNNER_SERVICE    Default: powerx-runner
  POWERX_SYNC_SYSTEMD_UNITS
                            Default: 1. Set to 0 when dev service unit files are managed separately.
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
PLUGIN_RUNTIME_ROOT="${POWERX_PLUGIN_RUNTIME_ROOT:-${LINKS_ROOT}/plugins}"
STORAGE_RUNTIME_ROOT="${POWERX_STORAGE_ROOT:-${LINKS_ROOT}/storage}"
HEALTH_URL="${POWERX_HEALTH_URL:-http://127.0.0.1:8080/api/v1/health}"
HEALTH_EXPECT="${POWERX_HEALTH_EXPECT:-200}"
SERVICE_USER="${POWERX_SERVICE_USER:-${SUDO_USER:-powerx}}"
SERVICE_GROUP="${POWERX_SERVICE_GROUP:-}"
BACKEND_SERVICE="${POWERX_BACKEND_SERVICE:-powerx-backend}"
WEB_ADMIN_SERVICE="${POWERX_WEB_ADMIN_SERVICE:-powerx-web-admin}"
RUNNER_SERVICE="${POWERX_RUNNER_SERVICE:-powerx-runner}"
SYNC_SYSTEMD_UNITS="${POWERX_SYNC_SYSTEMD_UNITS:-1}"
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
echo "[switch-release] services: backend=${BACKEND_SERVICE}, web-admin=${WEB_ADMIN_SERVICE}, runner=${RUNNER_SERVICE}"

service_working_dir() {
  local unit="$1"
  case "${unit}" in
    "${BACKEND_SERVICE}")
      printf "%s" "${LINKS_ROOT}/backend"
      ;;
    "${WEB_ADMIN_SERVICE}")
      printf "%s" "${LINKS_ROOT}/web-admin"
      ;;
    "${RUNNER_SERVICE}")
      printf "%s" "${LINKS_ROOT}/runner"
      ;;
    *)
      printf "%s" "${LINKS_ROOT}"
      ;;
  esac
}

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
  local working_dir
  working_dir="$(service_working_dir "${unit}")"
  install -d -m 0755 "$dir"
  cat > "${dir}/zz-runtime-user.conf" <<EOF
[Service]
User=${SERVICE_USER}
Group=${SERVICE_GROUP}
WorkingDirectory=${working_dir}
EOF
}

apply_setup_trace_override() {
  local dir="/etc/systemd/system/${BACKEND_SERVICE}.service.d"
  local file="${dir}/90-setup-trace.conf"
  install -d -m 0755 "$dir"
  cat > "$file" <<EOF
[Service]
Environment=POWERX_SETUP_STATUS_TRACE=1
EOF
  echo "[switch-release] setup trace enabled: ${file}"
}

remove_setup_trace_override() {
  local file="/etc/systemd/system/${BACKEND_SERVICE}.service.d/90-setup-trace.conf"
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
[switch-release] required by: ${WEB_ADMIN_SERVICE}.service / ${RUNNER_SERVICE}.service
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

sync_http_proxy_base_env() {
  local env_file="${RUNTIME_ROOT}/powerx.env"
  local proxy_base="http://127.0.0.1:8080"
  local current_value=""

  install -d -m 0755 "${RUNTIME_ROOT}"
  if [[ ! -f "$env_file" ]]; then
    touch "$env_file"
    chown root:root "$env_file"
    chmod 0644 "$env_file"
  fi

  if grep -q '^POWERX_HTTP_PROXY_BASE=' "$env_file"; then
    current_value="$(awk -F= '/^POWERX_HTTP_PROXY_BASE=/{print substr($0, index($0,$2)); exit}' "$env_file" | xargs)"
    if [[ -n "${current_value}" ]]; then
      echo "[switch-release] keep existing POWERX_HTTP_PROXY_BASE=${current_value}"
      return 0
    fi
    sed -i "s|^POWERX_HTTP_PROXY_BASE=.*$|POWERX_HTTP_PROXY_BASE=${proxy_base}|" "$env_file"
  else
    printf 'POWERX_HTTP_PROXY_BASE=%s\n' "${proxy_base}" >> "$env_file"
  fi

  chown root:root "$env_file"
  chmod 0644 "$env_file"
  echo "[switch-release] synced POWERX_HTTP_PROXY_BASE=${proxy_base}"
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

sync_runtime_plugin_paths() {
  if [[ ! -f "${RUNTIME_CONFIG_PATH}" ]]; then
    echo "[switch-release] warning: runtime config missing, skip plugin path sync: ${RUNTIME_CONFIG_PATH}" >&2
    return
  fi

  local plugin_installed_abs="${PLUGIN_RUNTIME_ROOT}/installed"
  local plugin_registry_abs="${PLUGIN_RUNTIME_ROOT}/registry.json"
  local legacy_installed_abs="${LINK_BACKEND}/plugins/installed"
  local legacy_registry_abs="${LINK_BACKEND}/plugins/registry.json"
  local tmp_file

  install -d -m 0755 "${plugin_installed_abs}"
  install -d -m 0755 "$(dirname "${plugin_registry_abs}")"

  # 从旧的 release 绑定路径迁移一次插件运行产物到持久目录（仅当目标为空时）。
  if [[ -d "${legacy_installed_abs}" ]]; then
    if [[ -z "$(find "${plugin_installed_abs}" -mindepth 1 -maxdepth 1 2>/dev/null | head -n 1)" ]]; then
      cp -a "${legacy_installed_abs}/." "${plugin_installed_abs}/" 2>/dev/null || true
      echo "[switch-release] plugin installed artifacts migrated: ${legacy_installed_abs} -> ${plugin_installed_abs}"
    fi
  fi

  if [[ ! -f "${plugin_registry_abs}" ]]; then
    if [[ -f "${legacy_registry_abs}" ]]; then
      cp -a "${legacy_registry_abs}" "${plugin_registry_abs}" 2>/dev/null || true
      echo "[switch-release] plugin registry migrated: ${legacy_registry_abs} -> ${plugin_registry_abs}"
    else
      printf '{}\n' > "${plugin_registry_abs}"
    fi
  fi
  chown -R "${SERVICE_USER}:${SERVICE_GROUP}" "${plugin_installed_abs}" "$(dirname "${plugin_registry_abs}")"
  chown "${SERVICE_USER}:${SERVICE_GROUP}" "${plugin_registry_abs}"
  chmod 0644 "${plugin_registry_abs}"

  if grep -Eq '^[[:space:]]*installed_dir[[:space:]]*:' "${RUNTIME_CONFIG_PATH}"; then
    tmp_file="$(mktemp "${RUNTIME_ROOT}/config.yaml.plugin-installed.XXXXXX")"
    awk -v val="${plugin_installed_abs}" '
      BEGIN { updated = 0 }
      /^[[:space:]]*installed_dir[[:space:]]*:/ && updated == 0 {
        indent = ""
        if (match($0, /^[[:space:]]*/)) {
          indent = substr($0, RSTART, RLENGTH)
        }
        print indent "installed_dir: " val
        updated = 1
        next
      }
      { print }
    ' "${RUNTIME_CONFIG_PATH}" > "${tmp_file}"
    mv "${tmp_file}" "${RUNTIME_CONFIG_PATH}"
  else
    echo "[switch-release] warning: key plugin.installed_dir not found in ${RUNTIME_CONFIG_PATH}, skip rewrite" >&2
  fi

  if grep -Eq '^[[:space:]]*registry_file[[:space:]]*:' "${RUNTIME_CONFIG_PATH}"; then
    tmp_file="$(mktemp "${RUNTIME_ROOT}/config.yaml.plugin-registry.XXXXXX")"
    awk -v val="${plugin_registry_abs}" '
      BEGIN { updated = 0 }
      /^[[:space:]]*registry_file[[:space:]]*:/ && updated == 0 {
        indent = ""
        if (match($0, /^[[:space:]]*/)) {
          indent = substr($0, RSTART, RLENGTH)
        }
        print indent "registry_file: " val
        updated = 1
        next
      }
      { print }
    ' "${RUNTIME_CONFIG_PATH}" > "${tmp_file}"
    mv "${tmp_file}" "${RUNTIME_CONFIG_PATH}"
  else
    echo "[switch-release] warning: key plugin.registry_file not found in ${RUNTIME_CONFIG_PATH}, skip rewrite" >&2
  fi

  chown "${SERVICE_USER}:${SERVICE_GROUP}" "${RUNTIME_CONFIG_PATH}"
  chmod 0644 "${RUNTIME_CONFIG_PATH}"
  echo "[switch-release] runtime plugin paths synced: installed_dir=${plugin_installed_abs} registry_file=${plugin_registry_abs}"
}

normalize_plugin_runtime_artifacts() {
  local plugin_installed_abs="${PLUGIN_RUNTIME_ROOT}/installed"
  local plugin_registry_abs="${PLUGIN_RUNTIME_ROOT}/registry.json"

  if [[ ! -d "${plugin_installed_abs}" ]]; then
    echo "[switch-release] warning: plugin installed dir missing, skip artifact normalize: ${plugin_installed_abs}" >&2
    return
  fi

  # 统一属主，避免切换后插件目录落成 root 导致运行用户不可读/不可执行。
  chown -R "${SERVICE_USER}:${SERVICE_GROUP}" "${plugin_installed_abs}"
  if [[ -f "${plugin_registry_abs}" ]]; then
    chown "${SERVICE_USER}:${SERVICE_GROUP}" "${plugin_registry_abs}"
    chmod 0644 "${plugin_registry_abs}"
  fi

  # 给插件后端可执行产物补执行位（migrate/plugin 等）。
  while IFS= read -r -d '' bin_dir; do
    find "${bin_dir}" -maxdepth 1 -type f -exec chmod 0755 {} \;
  done < <(find "${plugin_installed_abs}" -type d -path '*/backend/bin' -print0)

  echo "[switch-release] plugin runtime artifacts normalized under ${plugin_installed_abs}"
}

sync_runtime_storage_paths() {
  if [[ ! -f "${RUNTIME_CONFIG_PATH}" ]]; then
    echo "[switch-release] warning: runtime config missing, skip storage path sync: ${RUNTIME_CONFIG_PATH}" >&2
    return
  fi

  local media_abs="${STORAGE_RUNTIME_ROOT}/media"
  local legacy_paths=()
  local legacy
  local tmp_file

  install -d -m 0755 "${media_abs}"
  chown -R "${SERVICE_USER}:${SERVICE_GROUP}" "${STORAGE_RUNTIME_ROOT}"

  legacy_paths+=("${LINK_BACKEND}/storage/media")
  if [[ -n "${PREV_BACKEND}" ]]; then
    legacy_paths+=("${PREV_BACKEND}/storage/media")
  fi
  legacy_paths+=("${TARGET_BACKEND}/storage/media")

  for legacy in "${legacy_paths[@]}"; do
    if [[ -d "${legacy}" && "${legacy}" != "${media_abs}" ]]; then
      if [[ -n "$(find "${legacy}" -mindepth 1 -maxdepth 1 2>/dev/null | head -n 1)" ]]; then
        cp -a "${legacy}/." "${media_abs}/" 2>/dev/null || true
        echo "[switch-release] media storage migrated: ${legacy} -> ${media_abs}"
      fi
    fi
  done

  if grep -Eq '^[[:space:]]*base_path[[:space:]]*:' "${RUNTIME_CONFIG_PATH}"; then
    tmp_file="$(mktemp "${RUNTIME_ROOT}/config.yaml.storage.XXXXXX")"
    awk -v val="${media_abs}" '
      BEGIN { updated = 0; in_storage = 0; in_local = 0 }
      /^[^[:space:]][^:]*:[[:space:]]*$/ {
        in_storage = ($0 ~ /^storage[[:space:]]*:/)
        in_local = 0
      }
      in_storage && /^[[:space:]]{2}local[[:space:]]*:[[:space:]]*$/ {
        in_local = 1
      }
      in_storage && /^[[:space:]]{2}[A-Za-z0-9_]+[[:space:]]*:/ && $0 !~ /^[[:space:]]{2}local[[:space:]]*:/ {
        in_local = 0
      }
      in_storage && in_local && /^[[:space:]]*base_path[[:space:]]*:/ && updated == 0 {
        indent = ""
        if (match($0, /^[[:space:]]*/)) {
          indent = substr($0, RSTART, RLENGTH)
        }
        print indent "base_path: " val
        updated = 1
        next
      }
      { print }
    ' "${RUNTIME_CONFIG_PATH}" > "${tmp_file}"
    mv "${tmp_file}" "${RUNTIME_CONFIG_PATH}"
  else
    echo "[switch-release] warning: key storage.local.base_path not found in ${RUNTIME_CONFIG_PATH}, skip rewrite" >&2
  fi

  chown -R "${SERVICE_USER}:${SERVICE_GROUP}" "${media_abs}"
  chown "${SERVICE_USER}:${SERVICE_GROUP}" "${RUNTIME_CONFIG_PATH}"
  chmod 0644 "${RUNTIME_CONFIG_PATH}"
  echo "[switch-release] runtime media storage synced: base_path=${media_abs}"
}

sync_runtime_config_version() {
  if [[ ! -f "${RUNTIME_CONFIG_PATH}" ]]; then
    echo "[switch-release] warning: runtime config missing, skip version sync: ${RUNTIME_CONFIG_PATH}" >&2
    return
  fi

  local escaped_version
  local quoted_version
  local tmp_file
  escaped_version="${TARGET_REF//\\/\\\\}"
  escaped_version="${escaped_version//\"/\\\"}"
  quoted_version="\"${escaped_version}\""
  tmp_file="$(mktemp "${RUNTIME_ROOT}/config.yaml.switch.XXXXXX")"

  if grep -Eq '^[[:space:]]*version[[:space:]]*:' "${RUNTIME_CONFIG_PATH}"; then
    awk -v version_value="${quoted_version}" '
      BEGIN { updated = 0 }
      /^[[:space:]]*version[[:space:]]*:/ && updated == 0 {
        indent = ""
        if (match($0, /^[[:space:]]*/)) {
          indent = substr($0, RSTART, RLENGTH)
        }
        print indent "version: " version_value
        updated = 1
        next
      }
      { print }
    ' "${RUNTIME_CONFIG_PATH}" > "${tmp_file}"
  else
    {
      printf 'version: %s\n' "${quoted_version}"
      cat "${RUNTIME_CONFIG_PATH}"
    } > "${tmp_file}"
  fi

  mv "${tmp_file}" "${RUNTIME_CONFIG_PATH}"
  chown "${SERVICE_USER}:${SERVICE_GROUP}" "${RUNTIME_CONFIG_PATH}"
  chmod 0644 "${RUNTIME_CONFIG_PATH}"
  echo "[switch-release] runtime config version synced: ${RUNTIME_CONFIG_PATH} => ${TARGET_REF}"
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
  systemctl restart "${BACKEND_SERVICE}" "${WEB_ADMIN_SERVICE}"
  if [[ "$WITH_RUNNER" == "1" ]]; then
    systemctl restart "${RUNNER_SERVICE}"
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
sync_http_proxy_base_env
sync_runtime_storage_paths
sync_runtime_plugin_paths
normalize_plugin_runtime_artifacts
sync_runtime_config_version
set_setup_reentry_env

if [[ "$SYNC_SYSTEMD_UNITS" == "1" && -d "$TARGET_SYSTEMD" ]]; then
  cp "$TARGET_SYSTEMD"/*.service /etc/systemd/system/
elif [[ "$SYNC_SYSTEMD_UNITS" != "1" ]]; then
  echo "[switch-release] skip systemd unit sync: POWERX_SYNC_SYSTEMD_UNITS=${SYNC_SYSTEMD_UNITS}"
fi
apply_service_user_override "${BACKEND_SERVICE}"
apply_service_user_override "${WEB_ADMIN_SERVICE}"
if [[ "$WITH_RUNNER" == "1" ]]; then
  apply_service_user_override "${RUNNER_SERVICE}"
fi
if [[ "$WITH_SETUP_TRACE" == "1" ]]; then
  apply_setup_trace_override
fi
if [[ "$WITHOUT_SETUP_TRACE" == "1" ]]; then
  remove_setup_trace_override
fi

systemctl daemon-reload
assert_effective_service_user "${BACKEND_SERVICE}.service" "${SERVICE_USER}" "${SERVICE_GROUP}"
assert_effective_service_user "${WEB_ADMIN_SERVICE}.service" "${SERVICE_USER}" "${SERVICE_GROUP}"
if [[ "$WITH_RUNNER" == "1" ]]; then
  assert_effective_service_user "${RUNNER_SERVICE}.service" "${SERVICE_USER}" "${SERVICE_GROUP}"
fi
if [[ "$WITH_RUNNER" == "1" ]]; then
  systemctl enable "${BACKEND_SERVICE}" "${WEB_ADMIN_SERVICE}" "${RUNNER_SERVICE}"
else
  systemctl enable "${BACKEND_SERVICE}" "${WEB_ADMIN_SERVICE}"
fi
systemctl restart "${BACKEND_SERVICE}" "${WEB_ADMIN_SERVICE}"
if [[ "$WITH_RUNNER" == "1" ]]; then
  systemctl restart "${RUNNER_SERVICE}"
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
systemctl --no-pager --full status "${BACKEND_SERVICE}" "${WEB_ADMIN_SERVICE}" | sed -n '1,40p'
if [[ "$WITH_RUNNER" == "1" ]]; then
  systemctl --no-pager --full status "${RUNNER_SERVICE}" | sed -n '1,20p'
fi
