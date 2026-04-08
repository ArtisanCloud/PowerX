# Quickstart: PowerX 部署与运维治理基线（P0）

## 1. 前置条件

- 已在 `025-powerx-docker-systemd` 分支。
- 可访问 PostgreSQL、Redis、MinIO/S3。
- 已准备运维配置目录与日志目录。
- 端口默认策略已确认：
  - dev：`web-admin=3030`，`backend=8077`
  - prod：`web-admin=3000`，`backend=8080`
  - 优先级：环境变量（`POWERX_WEB_ADMIN_PORT`/`POWERX_BACKEND_PORT`）> setup 配置 > 配置默认值
  - 生效语义：修改 setup 端口后需要重启 backend/web-admin，重启前 `desired_ports` 可变更但 `effective_ports` 不变
- 安装状态机制：参考 `specs/025-powerx-docker-systemd/install-mechanism.md`（`config.install.status` 为首判定源）。
- 运行时配置真源：`/etc/powerx/config.yaml`（版本切换不覆盖）。

## 2. 部署基线验证

1. 按 `docs/plan/deploy/docker.md` 或 `systemd.md` 完成一次冷启动。
2. 首装场景确认系统进入 `/setup`，并完成安装流程（含 DB 连通性校验与初始化）。
3. 验证健康状态：

```bash
curl -f http://127.0.0.1:8077/api/v1/health
```

4. 验证插件入口与主站可访问。
5. 验证端口状态可观测：

```bash
curl -s http://127.0.0.1:8080/api/v1/admin/setup/status | jq '.data.desired_ports,.data.effective_ports,.data.restart_required'
```

## 2.1 已安装环境升级（不走 setup）

1. 构建并切换版本（仅代码升级）：

```bash
make dist DIST_VERSION=${POWERX_VERSION} NPM_INSTALL=0
sudo mv dist/systemd/${POWERX_VERSION} /opt/powerx/releases/
sudo bash backend/scripts/ops/switch-release-systemd.sh ${POWERX_VERSION}
```

2. 验证安装态保持 `installed`：

```bash
curl -s http://127.0.0.1:8080/api/v1/admin/setup/status | jq '.data.install_status,.data.restart_required'
```

3. 若本次版本包含 DB 变更，显式执行：

```bash
cd /opt/powerx/backend
./database migrate
# 仅在需要初始化/补数时执行
# ./database seed
```

## 3. 插件平滑升级演练

1. 执行“安装不启用”。
2. 执行版本切换。
3. 触发回滚并确认恢复。

参考：`docs/plan/deploy/plugin-upgrade-sop.md`

## 4. 备份恢复演练

1. 触发一次逻辑备份。
2. 执行一次清理任务（测试环境）。
3. 执行一次恢复演练并记录 RTO。

参考：`docs/plan/deploy/db-backup-job-templates.md`

## 5. 日志聚合验证

1. 确认 promtail 已采集目标日志源。
2. 在 Grafana Loki 中按 `service` 与 `plugin_id` 检索。
3. 触发一条可识别错误日志，确认告警链路。

参考：`docs/plan/deploy/logging-loki-grafana.md`

## 6. 管理页面 P0 验证

- Deploy 页面：可查看发布历史并执行回滚。
- Plugin 页面：可查看插件版本并执行切换。
- Backup 页面：可查看策略、任务、演练结果。
- Migration 页面：可触发 A->B runbook、提交验收、切换与回切。

参考：`docs/plan/deploy/management-console-p0-tasks.md`

## 7. Phase 7 验证记录（2026-03-25）

执行人：Codex（自动化实现与回归）

- 后端合同/集成回归（含 US1-US4 + traceability）：

```bash
cd backend && \
GOCACHE=$PWD/.gocache GOMODCACHE=$PWD/.gomodcache \
go test ./tests/contract/ops ./tests/integration/ops -count=1
```

- 预发布阻断脚本：

```bash
bash backend/scripts/ops/pre-release-gate.sh
```

- 覆盖率门禁：

```bash
bash backend/scripts/ci/coverage-gate.sh
```

- 性能烟测门禁（p95 < 200ms，针对 integration/ops 测试耗时）：

```bash
bash backend/scripts/ci/perf-smoke.sh
```

- 前端 E2E 回归：

```bash
cd web-admin && npm run test:e2e
```

- 一键执行 T080 回归（推荐）：

```bash
make test-full-regression
```

- 若当前环境无法监听本地端口（仅跑后端回归）：

```bash
make test-full-regression-backend
```

说明：若执行环境缺失 Playwright 浏览器依赖，E2E 会报环境错误；需先完成 `playwright install` 后重跑。
