# Knowledge Space Perf & Resiliency Validation

> 描述在 CI 或预发布环境执行批量入库、融合降级与反馈风暴测试的操作手册，用于满足 T057（性能 / 弹性验证）要求。

## 1. 环境准备

| 组件 | 要求 |
| --- | --- |
| PostgreSQL | 与生产一致的 `pgvector` 扩展，建议使用 4C/8G 实例 |
| Redis | 用于锁与事件队列，至少 1GB 内存 |
| Web Admin | `NUXT_FORCE_THEME` 可关闭，需启用 `knowledge-space-v1`、`knowledge-ingestion`、`fusion.pipeline`、`feedback.loop` feature flag |
| Grafana | 已导入 `Knowledge Space` / `fusion-pipeline` / `feedback-loop` dashboard |

> 报表目录约定：模块级快照在 `backend/reports/_state/*`，聚合总览在 `reports/_state/*`（见 `docs/ops/reports_layout.md`）。

### 1.1 推荐环境变量（预发布/本地服务）

```bash
export POWERX_BASE_URL="http://127.0.0.1:8077"
export ADMIN_TOKEN="<admin-jwt>"
export TENANT_UUID="<tenant-uuid>"
```

> 说明：仓库脚本会把 `POWERX_BASE_URL` 规范化为 `.../api/v1` 后再拼接路由；也兼容传入 `http://127.0.0.1:8077/api/v1`。

### 1.2 可选：开启 OCR / PDF 文本抽取（用于降级/修复演练）

默认 OCR 处理器不可用，会导致：
- `format=image` 或 `pdf + sourceUri 包含 scan` 时触发 **degraded / `ocr_unavailable`**
- `OCR required=true` 时触发 **blocked / `ocr_required`**

如需演练“安装依赖后恢复”的路径，推荐在 `etc/config.yaml` 显式配置（优先级高于自动探测）：

```yaml
knowledge_space:
  ingestion_processors:
    pdf_text_available: true
    ocr_available: true
```

依赖安装方式见：`docs/guides/deploy/knowledge_pdf_ocr.md:1`。

## 2. 批量入库压测

> 本节分两种执行方式：
> - **CI/本地合同测试**：不依赖已启动服务，适合验证服务内逻辑与快照生成（推荐作为基线）。
> - **预发布 HTTP 演练**：对已部署环境执行真实 API（适合压测/容量评估）。

### 2.1 CI/本地合同测试（不依赖已启动服务）

1. **启动 10 个并发入库**（每个进程独立 sqlite + stub vectorstore）：
   ```bash
   cd backend
   for n in $(seq 1 10); do
     go test ./tests/contract/knowledge_space -run TestTriggerIngestionHTTP -count=1 &
   done
   wait
   ```

2. **验证快照文件输出**（合同测试会写入 `tmp/test-runs/*`，不写入正式 `reports/_state/*`）：重点关注 `coveragePct/embeddingPct/maskingPct`、`degraded/errorCode/reason`。

### 2.2 预发布 HTTP 演练（需要已启动服务 + token）

1. **准备一个 spaceId**  
   - 方式 A：在 Web Admin 新建空间后复制 spaceId  
   - 方式 B：API 创建（示例依赖策略模版版本 ID）

2. **并发触发入库**（示例 10 并发）
   ```bash
   for n in $(seq 1 10); do
     curl -sS "$POWERX_BASE_URL/api/v1/admin/knowledge-spaces/<space-id>/ingestion-jobs" \
       -H "Authorization: Bearer $ADMIN_TOKEN" \
       -H "X-Tenant-UUID: $TENANT_UUID" \
       -H "Content-Type: application/json" \
       -d '{"format":"pdf","sourceUri":"s3://bucket/doc.pdf","priority":"high","requestedBy":"loadtest@powerx.local"}' \
       >/dev/null &
   done
   wait
   ```

3. **验证指标/快照**  
   - `reports/_state/knowledge-spaces.json` 中对应 space 的 `ingestion` 段是否更新  
   - Grafana `Knowledge Space` 看板：覆盖率、降级计数、入库耗时分布

## 3. 融合降级演练

1. **前置条件**：至少发布过一个融合策略（Vector/BM25 权重任意），且该 space 已有入库记录。

2. **模拟“回滚演练”**（基于脚本触发 rollback）
   ```bash
   POWERX_BASE_URL="$POWERX_BASE_URL" POWERX_TOKEN="$ADMIN_TOKEN" \
     node scripts/fusion/rollback_strategy.mjs <space-id> <strategy-id>
   ```

3. **模拟“源不可用导致降级/回滚”**（推荐方式）
   - **方式 A（最简单）**：在预发布环境临时让向量库 Health 失败（例如 pgvector DSN 错误、Milvus/Pinecone endpoint 不可达）。  
   - **方式 B（本地）**：未配置 `knowledge_space.vector_store.driver` 时，vector 源会被判定为 unavailable（会影响融合查询能力）。

4. **校验**  
   - Grafana `fusion-pipeline` 看板出现 `fusion.source.failed`（含 `degrade_reason`）  
   - `knowledge_audit_trail_entries` 中 action=`fusion.rollback`（若触发回滚）  
   - 记录回滚耗时（目标：≤5 分钟）

## 4. 反馈风暴测试

1. **模拟 50+ 条反馈**（HTTP，示例 60 并发）：
   ```bash
   for n in $(seq 1 60); do
     curl -sS "$POWERX_BASE_URL/api/v1/admin/knowledge-spaces/<space-id>/feedback" \
       -H "Authorization: Bearer $ADMIN_TOKEN" \
       -H "X-Tenant-UUID: $TENANT_UUID" \
       -H "Content-Type: application/json" \
       -d "{\"severity\":\"high\",\"issueType\":\"accuracy\",\"reportedBy\":\"loadtest@powerx.local\",\"linkedChunks\":[\"$(uuidgen)\"],\"notes\":\"load test case\"}" \
       >/dev/null &
   done
   wait
   ```
2. **确认限制**：`knowledge.feedback.reprocess` 事件仍按顺序发布，Redis/队列无堆积。
3. **SLA 审核**：`ListFeedbackCases` 返回的案例 `sla_due_at` 均大于当前时间，确保 24h SLA。

## 5. 知识更新链路（US6–US9）弹性演练

> 这些演练以“守卫触发/暂停/回滚”为目标，建议在预发布环境执行。

### 5.1 Delta（US6）— 版本治理

- 生成/校验输入：`node scripts/ops/knowledge-delta-job.mjs --space=<space-uuid> --source=default`
- 服务端执行建议（若已部署）：触发 delta job → publish → rollback（参见对应 HTTP/gRPC 接口）。
- 核对报表：`backend/reports/_state/knowledge-delta.json` 与 `reports/_state/knowledge-update.json` 的 `delta` 段。

### 5.2 Event（US7）— 热修幂等与重试

- 回放/查看最近事件：`node scripts/ops/knowledge-event-replay.mjs`
- 核对报表：`backend/reports/_state/knowledge-event.json` 与聚合 `event` 段。

### 5.3 Decay（US8）— 巡检与误判恢复

- Dry-run：`node scripts/ops/knowledge-decay-scan.mjs --dry-run --space=<space-uuid> --detected=3`
- 若接入环境：触发任务与恢复（参考 `docs/ops/gap_task_template.md`）。
- 核对报表：`backend/reports/_state/knowledge-decay.json` 与聚合 `decay` 段。

### 5.4 Release（US9）— 灰度扩散与回滚

- 校验矩阵：`node scripts/ops/knowledge-release-matrix.mjs --matrix=backend/config/knowledge/tenant_release_matrix.yaml`
- 若接入环境：publish → promote（传 alerts 触发暂停）→ rollback（参考 `docs/ops/release_guardrails.md`）。
- 核对报表：`backend/reports/_state/knowledge-release.json` 与聚合 `release` 段，确认 `versionDrift <= 1`。

## 6. 告警阈值与 SLO 建议

完成上述测试后，在 Grafana 中导出指标截图，并建议将以下阈值固化到告警配置（示例，需要按生产基线校准）：

- Ingestion：`coveragePct < 0.95`（P1），`job_duration > 4h`（P1）
- Fusion：`fusion.source.failed > 0` 且持续 5m（P1）
- Feedback：`open_cases > 30`（P2），`sla_breach > 0`（P1）
- Delta：`diff_accuracy < 98`（P1），`publish_latency > 30m`（P1）
- Event：`latency > 5m`（P1），`retry_count > 3`（P2）
- Decay：`false_positive_rate > 10%`（P2），`restore_latency > 10m`（P2）
- Release：`rollback_count >= 1`（P1），`version_drift > 1`（P1）

并将本次验证摘要写入冒烟报告（见 `docs/guides/knowledge_space/smoke_checklist.md`）。
