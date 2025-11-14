# Gap Task Template – Decay & Blank Governance

结合 `SCN-KNOWLEDGE-UPDATE-DECAY-001` 场景，巡检脚本与后台服务会根据阈值为每个知识空间生成“Gap Task”。该模板帮助 Ops/治理团队快速落地任务卡片、审批字段与恢复 SOP。

## 1. 任务字段（Task-Center Payload）

| 字段 | 类型 | 说明 | 示例 |
|------|------|------|------|
| `spaceId` | UUID | 触发任务的知识空间 | `87a4...` |
| `taskId` | UUID | 系统生成的任务 ID（可复用 API 返回值） | `task-47e8` |
| `category` | enum | `coverage` / `quality` / `blank` | `coverage` |
| `severity` | enum | `p1`/`p2`/`p3`，由 `configs/knowledge/decay_thresholds.yaml` 派生 | `p1` |
| `reason` | string | 阈值命中原因（引用率下降/评分过低等） | `引用跌破阈值` |
| `detectedAt` | ISO8601 | 巡检时间 | `2025-02-15T08:00:00Z` |
| `slaDueAt` | ISO8601 | 7 天 SLA（或配置值） | `2025-02-22T08:00:00Z` |
| `assignee` | string | 业务专家 / 内容供应方 | `ops.liang@powerx.io` |
| `approver` | string | 治理负责人，误判恢复必须填写 | `governance.xu@powerx.io` |
| `notes` | string | 处理/恢复补充说明 | `已回滚至 v2025.02` |

## 2. 审批与恢复字段

- **提交**：`reason`, `detectedAt`, `slaDueAt`, `assignee`，由脚本或服务自动填充。
- **审批**：`approver`, `approvalNotes`, `approvalAt`，审批中心要求误判恢复时必须给出理由。
- **恢复/误判**：`restoreAction`（`restore` / `rollback` / `dismiss`）、`restoreNotes`、`falsePositive`（bool）。
- **审计对接**：完成恢复后调用 `POST /knowledge/decay/restore`，API 会自动写入 `audit-ledger` 与 `knowledge-decay.json`。

## 3. CLI / 脚本

使用新脚本生成样例任务或导出报告：

```bash
node scripts/ops/knowledge-decay-scan.mjs \
  --space=87a4a0f0-9a7c-4a76-aa5f-2f7f3c9d1234 \
  --category=coverage --detected=3 --output=tmp/decay-report.json

# Dry-run（仅打印，不写文件）
node scripts/ops/knowledge-decay-scan.mjs --space=<uuid> --detected=2 --dry-run
```

输出结构：

```json
{
  "metadata": {
    "spaceId": "...",
    "threshold": "coverage",
    "severity": "p1"
  },
  "tasks": [
    {
      "taskId": "...",
      "category": "coverage",
      "slaDueAt": "2025-02-22T08:00:00Z"
    }
  ],
  "metrics": {
    "knowledge.decay.detected": 3,
    "knowledge.gap.backlog": 3
  }
}
```

## 4. 恢复 / 误判 SOP

1. 任务执行人完成补齐或确认误判 → 在任务中心更新 `resolution` + 附件。
2. 调用 `POST /knowledge/decay/restore`，带上 `taskId`, `notes`, `falsePositive`。
3. 系统会：
   - 关闭任务并写入 `knowledge.decay.false_positive` 指标；
   - 更新 `backend/reports/_state/knowledge-decay.json` 与汇总 `reports/_state/knowledge-update.json`；
   - 写入 `audit-ledger` 供合规追溯。
4. 若误判率 >10% 或 backlog >20，脚本/监控会触发 `PX_KNOWLEDGE_GAP_ALERT` 告警。

## 5. Dry-run Checklist

- [ ] 阈值文件 `configs/knowledge/decay_thresholds.yaml` 已合并最新版本号。
- [ ] `PX_KNOWLEDGE_DECAY_GUARD` / `PX_KNOWLEDGE_RESTORE_FLOW` feature flag 已开启。
- [ ] `node scripts/ops/knowledge-decay-scan.mjs --dry-run` 输出包含正确的 SLA、严重度与 metrics。
- [ ] `backend/reports/_state/knowledge-decay.json` 与 `reports/_state/knowledge-update.json` 更新时间 <5 分钟。
