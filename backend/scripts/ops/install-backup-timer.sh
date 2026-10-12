#!/usr/bin/env bash
set -euo pipefail
if [[ "${EUID}" != "0" ]]; then echo "请使用 sudo 执行" >&2; exit 1; fi
CORE_UNIT="${1:-powerx-backend}"
case "${CORE_UNIT}" in
  powerx-backend) BACKUP_UNIT=powerx-backup ;;
  powerx-dev-backend) BACKUP_UNIT=powerx-dev-backup ;;
  *) echo "仅支持 powerx-backend 或 powerx-dev-backend" >&2; exit 1 ;;
esac
if [[ "$(systemctl show "${CORE_UNIT}" -p LoadState --value)" != "loaded" ]]; then echo "Core 服务尚未安装" >&2; exit 1; fi
SCRIPT_DIR="$(cd -P -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
RELEASE_ROOT="$(cd -- "${SCRIPT_DIR}/../../.." && pwd)"
for file in "${BACKUP_UNIT}.service" "${BACKUP_UNIT}.timer"; do
  if [[ ! -f "${RELEASE_ROOT}/systemd/${file}" ]]; then echo "请使用 make dist 的完整发布包：缺少 ${file}" >&2; exit 1; fi
done
if ! "${RELEASE_ROOT}/backend/database" --version | grep -q '"backup-run"'; then echo "database 二进制缺少 backup-run，请切换到新发布包" >&2; exit 1; fi
SERVICE_USER="$(systemctl show "${CORE_UNIT}" -p User --value)"
SERVICE_USER="${SERVICE_USER:-root}"
SERVICE_GROUP="$(systemctl show "${CORE_UNIT}" -p Group --value)"
SERVICE_GROUP="${SERVICE_GROUP:-$(id -gn "${SERVICE_USER}")}"
install -m 0644 "${RELEASE_ROOT}/systemd/${BACKUP_UNIT}.service" "/etc/systemd/system/${BACKUP_UNIT}.service"
install -m 0644 "${RELEASE_ROOT}/systemd/${BACKUP_UNIT}.timer" "/etc/systemd/system/${BACKUP_UNIT}.timer"
mkdir -p "/etc/systemd/system/${BACKUP_UNIT}.service.d"
printf '[Service]\nUser=%s\nGroup=%s\n' "${SERVICE_USER}" "${SERVICE_GROUP}" > "/etc/systemd/system/${BACKUP_UNIT}.service.d/runtime-user.conf"
chmod 0644 "/etc/systemd/system/${BACKUP_UNIT}.service.d/runtime-user.conf"
systemctl daemon-reload
systemctl enable --now "${BACKUP_UNIT}.timer"
systemctl list-timers "${BACKUP_UNIT}.timer" --no-pager
echo "独立备份 timer 已启用；使用 ${CORE_UNIT} 相同的用户、配置和环境文件。请在备份中心启用策略并验证首份备份。"
