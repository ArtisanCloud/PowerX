# Knowledge Space Perf & Resiliency Validation

> 描述在 CI 或预发布环境执行批量入库、融合降级与反馈风暴测试的操作手册，用于满足 T057（性能 / 弹性验证）要求。

## 1. 环境准备

| 组件 | 要求 |
| --- | --- |
| PostgreSQL | 与生产一致的 `pgvector` 扩展，建议使用 4C/8G 实例 |
| Redis | 用于锁与事件队列，至少 1GB 内存 |
| Web Admin | `NUXT_FORCE_THEME` 可关闭，需启用 `knowledge-space-v1`、`knowledge-ingestion`、`fusion.pipeline`、`feedback.loop` feature flag |
| Grafana | 已导入 `Knowledge Space` / `fusion-pipeline` / `feedback-loop` dashboard |

## 2. 批量入库压测

1. **启动 10 个并发入库**：
   ```bash
   for n in $(seq 1 10); do
     go test ./tests/contract/knowledge_space -run TestTriggerIngestionHTTP -count=1 &
   done
   wait
   ```
2. **验证指标**：Grafana 覆盖率维持 ≥95%，`reports/_state/knowledge-spaces.json` 中的 `ingestion.coveragePct`=100。
3. **告警校验**：临时降低 `knowledge.ingestion.latency` 告警阈值，确认触发后再恢复默认值（4h SLA）。

## 3. 融合降级演练

1. **模拟外部 API 失败**：将 `vectorStore.Query` 替换为返回 `ErrNoRows`（或在测试环境关闭向量库）。
2. **触发策略**：
   ```bash
   node scripts/fusion/rollback_strategy.mjs <space-id> <strategy-id>
   ```
3. **校验**：Grafana `fusion-pipeline` 看板出现 `fusion.source.failed`，5 分钟内完成回滚。调回原配置并记录处理时间。

## 4. 反馈风暴测试

1. **模拟 50+ 条反馈**：
   ```bash
   for n in $(seq 1 60); do
     http POST :8080/api/admin/knowledge-spaces/<space-id>/feedback \
       severity=high issueType=accuracy reportedBy="loadtest@powerx.io" \
       linkedChunks:='["'$(uuidgen)'"]' notes='load test case' > /dev/null &
   done
   wait
   ```
2. **确认限制**：`knowledge.feedback.reprocess` 事件仍按顺序发布，Redis/队列无堆积。
3. **SLA 审核**：`ListFeedbackCases` 返回的案例 `sla_due_at` 均大于当前时间，确保 24h SLA。

## 5. 报表与告警调整

完成上述测试后，在 Grafana 中导出指标截图，并：

- 将 `feedback-loop` dashboard 的告警阈值设置为 `open_cases > 30` 触发 warning。
- 更新 `docs/guides/knowledge_space/smoke_checklist.md` 的“最近一次验证时间”字段。
