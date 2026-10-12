#!/usr/bin/env bash
set -euo pipefail
# 调用者显式选择部署实例；仅安装归档单元，不重启 Core 或 PostgreSQL。
PX_ARCHIVE_ENV="${1:-}"
if [[ "$PX_ARCHIVE_ENV" != "prod" && "$PX_ARCHIVE_ENV" != "dev" ]]; then echo 'usage: sudo install-archive-timer.sh <prod|dev>' >&2; exit 2; fi
if [[ "$EUID" != 0 ]]; then echo 'root privileges required to install systemd units' >&2; exit 1; fi
if [[ "$PX_ARCHIVE_ENV" == dev ]]; then
  PX_ARCHIVE_ROOT=/opt/powerx-dev; PX_ARCHIVE_ETC=/etc/powerx-dev; PX_ARCHIVE_UNIT=powerx-dev-archive
else
  PX_ARCHIVE_ROOT=/opt/powerx; PX_ARCHIVE_ETC=/etc/powerx; PX_ARCHIVE_UNIT=powerx-archive
fi
if [[ ! -x "$PX_ARCHIVE_ROOT/backend/archive-worker" ]]; then echo 'archive-worker missing; deploy the complete dist first' >&2; exit 1; fi
if [[ ! -r "$PX_ARCHIVE_ETC/powerx.env" ]]; then echo 'instance EnvironmentFile missing' >&2; exit 1; fi
PX_ARCHIVE_SYSTEMD_DIR="$PX_ARCHIVE_ROOT/systemd"
if [[ ! -f "$PX_ARCHIVE_SYSTEMD_DIR/$PX_ARCHIVE_UNIT.service" ]]; then
  PX_ARCHIVE_SCRIPT_DIR="$(cd -P "$(dirname "$0")" && pwd -P)"
  PX_ARCHIVE_RELEASE_ROOT="$(cd -P "$PX_ARCHIVE_SCRIPT_DIR/../../.." && pwd -P)"
  if [[ -f "$PX_ARCHIVE_RELEASE_ROOT/systemd/$PX_ARCHIVE_UNIT.service" ]]; then
    PX_ARCHIVE_SYSTEMD_DIR="$PX_ARCHIVE_RELEASE_ROOT/systemd"
  else
    PX_ARCHIVE_SYSTEMD_DIR="$PX_ARCHIVE_RELEASE_ROOT/deploy/powerx/systemd"
  fi
fi
install -m 0644 "$PX_ARCHIVE_SYSTEMD_DIR/$PX_ARCHIVE_UNIT.service" /etc/systemd/system/
install -m 0644 "$PX_ARCHIVE_SYSTEMD_DIR/$PX_ARCHIVE_UNIT.timer" /etc/systemd/system/
systemctl daemon-reload
systemctl enable --now "$PX_ARCHIVE_UNIT.timer"
echo "installed $PX_ARCHIVE_UNIT.timer; inspect with systemctl status $PX_ARCHIVE_UNIT.timer"
