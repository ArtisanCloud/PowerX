#!/usr/bin/env bash
set -euo pipefail

# 执行备份清理。
# 说明:
# - 具体“保留最近 N 份”的策略由服务层计算并落库；
# - 脚本负责外部资源侧的清理动作（对象存储/文件系统）。
echo "[cleanup-backups] start at $(date -u +%FT%TZ)"
# placeholder: prune expired backup artifacts
sleep 0.05
echo "[cleanup-backups] done"
