# Tenant Release Guardrails（租户灰度发布守卫）

对应：`SCN-KNOWLEDGE-UPDATE-TENANT-001` / `UC-KNOWLEDGE-UPDATE-TENANT-001`

## 组件说明（服务端 / 管理界面 / 辅助工具）

- **服务端（后端主进程）**：由 `backend/cmd/app/main.go` 启动，提供灰度发布相关 HTTP/gRPC 能力（例如 `/api/knowledge/release/*` 与 gRPC Release RPC）。
- **管理界面（Web Admin）**：`web-admin/app/pages/knowledge-spaces/release.vue`，通过 HTTP API 展示策略、指标、批次推进与回滚操作。
- **辅助工具（CLI）**：`backend/cmd/knowledge/release.go` 是命令行工具（客户端），通过 gRPC 调用服务端的 release 接口（upsert/publish/promote/rollback 等）；它**不会**随 `backend/cmd/app/main.go` 一起启动，只有在你显式执行它时才会运行。

## 目标

- 灰度发布按租户矩阵分批推进，指标异常时自动暂停并可在 ≤5 分钟内回滚。
- 确保 `version drift ≤ 1`（同一策略下同时活跃的版本不超过 2 个）。
- 所有动作（策略变更 / 发布 / 暂停 / 回滚）必须写入审计（audit-ledger）。

## Feature Flags（CI/ops 可控）

- `PX_TENANT_RELEASE_MATRIX`：允许策略矩阵管理（`/knowledge/release/policies`）。
- `PX_KNOWLEDGE_GRAY_RELEASE`：允许 publish/promote/status 等灰度流程 API。
- `PX_KNOWLEDGE_RELEASE_GUARD`：回滚守卫（关闭时默认拒绝 `rollback`，可通过请求头 `X-Bypass-Guard` 在受控场景绕过）。

说明：与其它模块保持一致——环境变量未设置视为启用；显式设置为 `0/false/disabled/off/no` 视为禁用。

## 指标门槛（示例）

建议在 `backend/config/knowledge/tenant_release_matrix.yaml` 的 `guardrails` 中声明（实际阈值由运维/治理团队校准）：

- `latency_p95`：`<5m`
- `rollback_sla_minutes`：`5`
- `error_rate`：`<5%`
- `version_drift`：`<=1`

## 操作剧本

### 1) 策略校验与写入

- 校验矩阵：`node scripts/ops/knowledge-release-matrix.mjs --matrix=backend/config/knowledge/tenant_release_matrix.yaml`
- 写入策略（HTTP）：`node scripts/ops/knowledge-release-matrix.mjs --base-url=$POWERX_BASE_URL --token=$ADMIN_TOKEN --tenant-uuid=$TENANT_UUID`

### 2) 发布与扩散

- 发布并自动推进：
  - `node scripts/ops/knowledge-release-matrix.mjs --base-url=$POWERX_BASE_URL --token=$ADMIN_TOKEN --tenant-uuid=$TENANT_UUID --publish=ver-2025.02 --auto-promote=true`

### 3) 异常暂停与回滚

- promote 时传入 `alerts` 会将批次置为 `paused` 并触发审计事件 `knowledge.release.pause`。
- 回滚：
  - HTTP：`POST /api/knowledge/release/rollback`（需 `PX_KNOWLEDGE_RELEASE_GUARD`）
  - gRPC：`RollbackRelease`

### 4) 报表与审计核对

- 报表：`backend/reports/_state/knowledge-release.json`
- 聚合：`reports/_state/knowledge-update.json`（`release` 段）

> 报表目录约定见：`docs/ops/reports_layout.md`
