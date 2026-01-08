# Gap/Decay 任务模板（Knowledge Decay & Gap Guard）

用于 `SCN-KNOWLEDGE-UPDATE-DECAY-001`：巡检 → 任务派发 → 补齐/恢复 → 审批 → 指标回写。

## 任务类型

- `decay.remediation`：衰减治理（引用下降/投诉上升/质量退化）
- `gap.fill`：知识空白补齐（主题/领域覆盖缺失）
- `restore.false_positive`：误判恢复（撤销误报、回滚补齐失败）

## 任务字段（建议与 task-center 对齐）

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `task_id` | string(uuid) | 是 | 任务唯一 ID |
| `space_id` | string(uuid) | 是 | 知识空间 ID |
| `tenant_uuid` | string(uuid) | 是 | 租户 UUID（用于隔离与统计） |
| `category` | string | 是 | `coverage`/`quality`/`blank` |
| `severity` | string | 是 | `p1`/`p2`/`p3` |
| `detected_at` | string(RFC3339) | 是 | 巡检检测时间 |
| `sla_due_at` | string(RFC3339) | 是 | SLA 截止时间（默认 7 天，可按阈值调整） |
| `assigned_to` | string | 否 | 派发执行人/小组 |
| `requires_approval` | bool | 是 | 是否需要审批（默认 true） |
| `reason` | string | 是 | 触发原因（阈值命中/趋势说明/空白描述） |
| `evidence` | object | 否 | 证据快照（引用/反馈/失败率/更新时间等） |
| `labels` | string[] | 否 | 业务标签（法规/财务/供应链等） |

## 审批字段（误判恢复/回滚必填）

误判恢复与回滚路径必须写入审计（audit-ledger）并可追溯：

- `approved_by`：审批人（从请求上下文 subject 获取；若无则显式传入）
- `approval_reason`：误判原因/回滚原因（禁止空）
- `approved_at`：审批时间

## 恢复/误判剧本（≤10 分钟）

1. 在任务卡片中选择 `restore.false_positive`（或在补齐任务失败后选择回滚）。
2. 填写 `approved_by` 与 `approval_reason`（必填），并关联原 `task_id`。
3. 调用 `POST /api/knowledge/decay/restore`（或 gRPC `RestoreDecayTask`）完成关闭与回写审计。
4. 验证：
   - 任务状态变更为 `closed`
   - `knowledge.decay.false_positive` 递增
   - `knowledge.gap.backlog` 递减
   - `reports/_state/knowledge-decay.json` 与 `reports/_state/knowledge-update.json` 的 `decay` 段更新

## Dry-run 与报告导出

- Dry-run：`node scripts/ops/knowledge-decay-scan.mjs --dry-run --space=<space_uuid> --detected=3`
- 触发巡检（创建任务 + 读取本地报表）：`node scripts/ops/knowledge-decay-scan.mjs --base-url=$POWERX_BASE_URL --token=$ADMIN_TOKEN --tenant-uuid=$TENANT_UUID --space=<space_uuid> --detected=3`
- 导出 CSV：`GET /api/knowledge/decay/status?export=csv`（自动按租户上下文隔离）
