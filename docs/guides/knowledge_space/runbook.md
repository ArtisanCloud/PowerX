# Knowledge Space Runbook

> 适用于运维与 SRE 团队，指导如何排查/恢复知识空间的入库、融合与反馈链路。所有命令均在仓库根目录执行，默认使用本地 `backend/etc/config.yaml`。

---

## 1. 快速定位

| 场景 | 入口 | 关键指标/日志 |
| --- | --- | --- |
| 入库异常/长时间运行 | Grafana → `Knowledge Space` 看板 | `knowledge.ingestion.*` event、`reports/_state/knowledge-spaces.json` 中的 `ingestion` 段 |
| 融合策略降级/冲突 | Grafana → `fusion-pipeline` 看板 | `fusion.source.failed` 事件、`scripts/fusion/rollback_strategy.mjs` 运行结果 |
| 反馈 SLA 超时 | Grafana → `feedback-loop` 看板 | `reports/_state/knowledge-spaces.json` 中的 `feedback` 段、`knowledge.feedback.reprocess` 事件 |
| 增量同步/版本发布异常（US6） | Grafana → `Knowledge Delta Sync` 看板 | `reports/_state/knowledge-update.json` 的 `delta` 段、`backend/reports/_state/knowledge-delta.json` |
| 事件热修异常（US7） | Grafana → `Event Hotfix` 看板 | `reports/_state/knowledge-update.json` 的 `event` 段、`backend/reports/_state/knowledge-event.json` |
| 衰减巡检/空白治理（US8） | Grafana → `Knowledge Decay Monitor` 看板 | `reports/_state/knowledge-update.json` 的 `decay` 段、`backend/reports/_state/knowledge-decay.json` |
| 租户灰度发布（US9） | Grafana → `Tenant Release Control` 看板 | `reports/_state/knowledge-update.json` 的 `release` 段、`backend/reports/_state/knowledge-release.json` |

---

## 2. 入库恢复流程

1. **确认任务状态**  
   ```bash
   cd backend && go test ./tests/contract/knowledge_space -run TestTriggerIngestionHTTP
   ```  
   若失败，参考 `internal/service/knowledge_space/ingestion_service.go` 日志。

2. **重新触发**  
   ```bash
   http POST :8077/api/v1/admin/knowledge-spaces/<space-id>/ingestion-jobs \
     sourceType=pdf sourceUri=s3://bucket/doc.pdf priority=high
   ```
   OCR/Processor 常见分支：
   - **blocked / `errorCode=ocr_required`**：当前文档需要 OCR，但后端 OCR 能力不可用。先确认系统依赖已安装（`tesseract` + `pdftoppm` 或 `mutool`），并在 `etc/config.yaml` 设置 `knowledge_space.ingestion_processors.ocr_available: true` 后重跑；或在 UI 的入库高级设置里关闭 `OCR required`（会降级）。
   - **completed + `errorCode=degraded` / `reason=ocr_unavailable`**：OCR 不可用导致降级，可能出现内容为空/引用覆盖下降；按上面步骤启用 OCR 后重跑入库。

3. **校验指标**  
   - `cat reports/_state/knowledge-spaces.json | jq '."space-id".ingestion'`
   - Grafana `Knowledge Space` 面板的覆盖率/嵌入率是否回到 100%

4. **告警与记录**  
   通过 `knowledge_space` 域的 IM Webhook（`backend/etc/config.yaml` → `knowledge_space.notifications.im_webhook`）发送处理摘要。

---

## 3. 融合策略与回滚

1. **查看冲突队列**（HTTP）
   ```bash
   http GET :8077/api/v1/admin/knowledge-spaces/<space-id>/fusion-strategies
   ```

2. **CLI 回滚**  
   ```bash
   POWERX_BASE_URL="http://127.0.0.1:8077" POWERX_TOKEN="$ADMIN_TOKEN" \
     node scripts/fusion/rollback_strategy.mjs <space-id> <strategy-id>
   ```
   输出 `strategyId`、`deploymentState` 后在 Grafana `fusion-pipeline` 看板确认降级已解除。

3. **事件/审计**  
   - 事件总线：`knowledge.space.fusion`  
   - 审计表：`knowledge_audit_trail_entries`（action=`fusion.rollback`）

---

## 4. 反馈与再加工

1. **提交或复现反馈**  
   ```bash
   http POST :8077/api/v1/admin/knowledge-spaces/<space-id>/feedback \
     severity=high issueType=accuracy reportedBy=ops@powerx.local \
     linkedChunks:='["<chunk-uuid>"]' notes='answer incorrect'
   ```

2. **监控再加工任务**  
   - 事件：`knowledge.feedback.reprocess`
   - `reports/_state/knowledge-spaces.json | jq '."space-id".feedback'`

3. **批量查询**（gRPC）
   ```bash
   grpcurl -d '{"space_id":"<space-id>"}' \
     -plaintext localhost:9001 \
     powerx.knowledge.v1.KnowledgeSpaceAdminService/ListFeedbackCases
   ```

4. **SLA 超时时的处理**  
   - 若 `case.status` 仍为 `in_progress` 且 `sla_due_at < now()`：  
     1. 手动触发 reprocess（复用上方 HTTP 请求）  
     2. 将案例状态更新为 `escalated`，通知 Ops 群组  
     3. 在 `docs/guides/knowledge_space/smoke_checklist.md` 记录本次处理摘要

---

## 5. 知识更新链路（US6–US9）常用脚本

> 这些脚本用于“巡检/对齐配置/导出报告”，不替代后端服务。报表目录约定见 `docs/ops/reports_layout.md`。

- Delta（US6）：`node scripts/ops/knowledge-delta-job.mjs --space=<space-uuid> --source=default`
- Event（US7）：`node scripts/ops/knowledge-event-replay.mjs`
- Decay（US8）：`node scripts/ops/knowledge-decay-scan.mjs --dry-run --space=<space-uuid> --detected=3`
- Release（US9）：`node scripts/ops/knowledge-release-matrix.mjs --matrix=backend/config/knowledge/tenant_release_matrix.yaml`

---

## 6. 参考资料

- [Quickstart — Knowledge Space Provisioning & Lifecycle Governance](../../specs/011-knowledge-space/quickstart.md)
- [UI 使用指南](./ui_guide.md)
- [Perf & Resiliency Validation](./perf_validation.md)
- [Smoke Checklist](./smoke_checklist.md)
