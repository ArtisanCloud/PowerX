# Knowledge Space Runbook

> 适用于运维与 SRE 团队，指导如何排查/恢复知识空间的入库、融合与反馈链路。所有命令均在仓库根目录执行，默认使用本地 `config/config.yaml`。

---

## 1. 快速定位

| 场景 | 入口 | 关键指标/日志 |
| --- | --- | --- |
| 入库异常/长时间运行 | Grafana → `Knowledge Space` 看板 | `knowledge.ingestion.*` event、`reports/_state/knowledge-spaces.json` 中的 `ingestion` 段 |
| 融合策略降级/冲突 | Grafana → `fusion-pipeline` 看板 | `fusion.source.failed` 事件、`scripts/fusion/rollback_strategy.mjs` 运行结果 |
| 反馈 SLA 超时 | Grafana → `feedback-loop` 看板 | `reports/_state/knowledge-spaces.json` 中的 `feedback` 段、`knowledge.feedback.reprocess` 事件 |

---

## 2. 入库恢复流程

1. **确认任务状态**  
   ```bash
   go test ./tests/contract/knowledge_space -run TestTriggerIngestionHTTP
   ```  
   若失败，参考 `internal/service/knowledge_space/ingestion_service.go` 日志。

2. **重新触发**  
   ```bash
   http POST :8080/api/admin/knowledge-spaces/<space-id>/ingestion-jobs \
     sourceType=pdf sourceUri=s3://bucket/doc.pdf priority=high
   ```

3. **校验指标**  
   - `cat reports/_state/knowledge-spaces.json | jq '."space-id".ingestion'`
   - Grafana `Knowledge Space` 面板的覆盖率/嵌入率是否回到 100%

4. **告警与记录**  
   通过 `knowledge_space` 域的 IM Webhook（`config/config.yaml` → `knowledge_space.notifications.im_webhook`）发送处理摘要。

---

## 3. 融合策略与回滚

1. **查看冲突队列**（HTTP）
   ```bash
   http GET :8080/api/admin/knowledge-spaces/<space-id>/fusion-strategies
   ```

2. **CLI 回滚**  
   ```bash
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
   http POST :8080/api/admin/knowledge-spaces/<space-id>/feedback \
     severity=high issueType=accuracy reportedBy=ops@powerx.local \
     linkedChunks:='["<chunk-uuid>"]' notes='answer incorrect'
   ```

2. **监控再加工任务**  
   - 事件：`knowledge.feedback.reprocess`
   - `reports/_state/knowledge-spaces.json | jq '."space-id".feedback'`

3. **批量查询**（gRPC）
   ```bash
   grpcurl -d '{"space_id":"<space-id>"}' \
     -plaintext localhost:8080 \
     powerx.knowledge.v1.KnowledgeSpaceAdminService/ListFeedbackCases
   ```

4. **SLA 超时时的处理**  
   - 若 `case.status` 仍为 `in_progress` 且 `sla_due_at < now()`：  
     1. 手动触发 reprocess（复用上方 HTTP 请求）  
     2. 将案例状态更新为 `escalated`，通知 Ops 群组  
     3. 在 `docs/guides/knowledge_space/smoke_checklist.md` 记录本次处理摘要

---

## 5. 参考资料

- [Quickstart — Knowledge Space Provisioning & Lifecycle Governance](../../specs/011-knowledge-space/quickstart.md)
- [Perf & Resiliency Validation](./perf_validation.md)
- [Smoke Checklist](./smoke_checklist.md)
