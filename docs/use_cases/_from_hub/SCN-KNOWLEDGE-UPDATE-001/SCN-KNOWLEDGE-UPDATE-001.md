---
scn_id: SCN-KNOWLEDGE-UPDATE-001
title: 知识更新与反馈
status: Draft
version: v0.1.0
owners:
  - name: Michael Hu
    role: Product Manager
    contact: matrix-x@artisan-cloud.com
domains: [knowledge]
layers: [service, data, ops]
repos:
  - key: powerx-core
    scope: knowledge-update
    responsibility: 增量同步、反馈再加工、事件驱动与衰减巡检工作流
  - key: powerx-core
    scope: knowledge-governance
    responsibility: 版本审批、灰度发布、审计与告警
related_usecases:
  - doc_id: UC-KNOWLEDGE-UPDATE-SYNC-001
    layer: service
    domain: knowledge
  - doc_id: UC-KNOWLEDGE-UPDATE-FEEDBACK-001
    layer: data
    domain: knowledge
  - doc_id: UC-KNOWLEDGE-UPDATE-EVENT-001
    layer: service
    domain: knowledge
  - doc_id: UC-KNOWLEDGE-UPDATE-DECAY-001
    layer: ops
    domain: knowledge
  - doc_id: UC-KNOWLEDGE-UPDATE-TENANT-001
    layer: ops
    domain: knowledge
last_reviewed_at: 2025-02-14

---

# Positioning & Goals

> 知识更新与反馈场景聚焦“增量同步 → 反馈驱动再加工 → 实时事件刷新 → 衰减巡检 → 灰度发布”的闭环，让知识空间在高频变化的业务中保持可信、可追溯、可治理。

目标体系：
- 增量包 30 分钟内处理完成，版本差异对比准确率 ≥ 98%。
- 用户反馈 24 小时闭环且修复后同类问题准确率提升 ≥ 25%。
- 关键事件 5 分钟内完成刷新并同步至 Agent/工作流。
- 知识衰减巡检覆盖率 100%，空白/低质内容可追溯并可恢复。
- 租户灰度发布与回滚过程可审计、可度量，确保敏感租户安全。

# Scope & Guardrails

- **In Scope**：增量同步、版本管理、反馈采集与再加工、事件驱动刷新、衰减检测、灰度发布/回滚、审计与告警。
- **Out of Scope**：知识初次入库（由 `SCN-KNOWLEDGE-SPACE-001` 负责）、插件/Agent 对知识的消费 UI、模型微调。
- **Environment & Flags**：`PX_KNOWLEDGE_DELTA_SYNC`, `PX_KNOWLEDGE_FEEDBACK_LOOP`, `PX_KNOWLEDGE_EVENT_HOTFIX`, `PX_KNOWLEDGE_DECAY_GUARD`, `PX_KNOWLEDGE_GRAY_RELEASE`，并依赖 `knowledge-space`, `ingestion-orchestrator`, `feedback-service`, `audit-ledger`, `event-bus`。

# Participants & Responsibilities

| Scope | Repository | Layer | 责任与交付物 | Owners |
|-------|------------|-------|--------------|--------|
| Delta Sync & Versioning | powerx-core | service | 增量抓取、差异报告、版本审批、回滚 | Knowledge Platform Squad |
| Feedback Intelligence | powerx-core | data | 反馈采集、再加工、质量评分、SLA 指标 | Knowledge Ops Squad |
| Event Hotfix Engine | powerx-core | service | 事件订阅、热更新、幂等控制、Agent 通知 | Core Platform Squad |
| Decay & Gap Monitor | powerx-core | ops | 巡检脚本、空白提醒、恢复流程 | Reliability Squad |
| Tenant Release Control | powerx-core | ops | 灰度策略、租户版本对齐、审计/告警 | Governance Squad |

# Core Capabilities

1. **Delta Sync & Versioning**：多源增量抓取、差异报告、审批与版本血缘，支持快速回滚。
2. **Feedback-driven Reprocessing**：把用户/Agent 反馈转换为再加工任务，自动重切片、重索引并验证质量提升。
3. **Event-driven Hot Refresh**：实时事件触发轻量更新流程，保证政策/价格等高优内容同步。
4. **Decay & Gap Detection**：通过巡检指标识别低质量或缺失知识，自动生成补齐/恢复任务。
5. **Tenant-aware Release & Governance**：灰度发布、租户分批对齐、失败回滚与全链路审计。

# End-to-End Flow

1. **Stage 1 – Detect & Collect**：调度、事件或反馈入口侦测变更，生成增量包/工单/事件。
2. **Stage 2 – Assess & Approve**：差异报告、敏感校验、风险评估，必要时触发审批与灰度策略。
3. **Stage 3 – Apply & Validate**：执行再加工/刷新/热更新，构建索引并热切换，运行自动化验证与回归脚本。
4. **Stage 4 – Observe & Iterate**：记录审计、指标与告警，若 SLA 未达标，自动回滚或升级人工干预。

# Key Interactions & Contracts

- **APIs / Jobs**：`POST /knowledge/delta/start`, `GET /knowledge/delta/reports/:id`, `POST /knowledge/feedback`, `POST /knowledge/reprocess`, `POST /knowledge/events/apply`, `POST /knowledge/gray-release`, `POST /audit/logs`。
- **Events**：`knowledge.delta.generated`, `knowledge.feedback.created`, `knowledge.event.received`, `knowledge.decay.detected`, `knowledge.release.state_changed`。
- **Configs**：`backend/config/knowledge/delta.yaml`, `feedback_playbook.yaml`, `event_hotfix_policies.yaml`, `decay_thresholds.yaml`, `tenant_release_matrix.yaml`。

# Validation Workflow

1. **增量同步链路**：模拟外部文档更新→delta 报告→审批→版本切换→回滚，验证差异准确率与审计记录。
2. **反馈再加工链路**：批量注入回答错误反馈，观察任务分配、再切分脚本、重索引及 SLA 告警。
3. **事件热更新链路**：推送法规/价格事件，确认 5 分钟内完成刷新、Agent 引用更新内容。
4. **衰减巡检链路**：运行巡检脚本，验证空白/误判恢复流程与指标回写。
5. **灰度发布链路**：对试点租户发布新版本，若指标不达标则自动回滚并通知治理团队。

# Related Links

- 子场景：`SCN-KNOWLEDGE-UPDATE-SYNC-001`, `SCN-KNOWLEDGE-UPDATE-FEEDBACK-001`, `SCN-KNOWLEDGE-UPDATE-EVENT-001`, `SCN-KNOWLEDGE-UPDATE-DECAY-001`, `SCN-KNOWLEDGE-UPDATE-TENANT-001`。
- 依赖场景：`SCN-KNOWLEDGE-SPACE-001`（构建与入库基础）、`SCN-KNOWLEDGE-RAG-FEEDBACK-001`（RAG 反馈指标）。
- Meta 参考：`docs/meta/scenarios/powerx/agent-and-automation/knowledge-and-reasoning/knowledge-update-and-feedback/primary.md`。

# Acceptance Criteria

1. 增量包从检测到发布 ≤ 30 分钟，版本差异准确率 ≥ 98%，支持单键回滚。
2. 反馈闭环 SLA ≤ 24 小时，修复后相同问题准确率提升 ≥ 25%，反馈积压自动告警。
3. 关键事件处理延迟 ≤ 5 分钟，事件幂等策略阻止重复更新，Agent 下一轮回答引用最新内容。
4. 衰减巡检覆盖率 100%，误判恢复 ≤ 10 分钟，空白任务自动指派。
5. 灰度发布具备租户级审计与回滚，失败时自动暂停扩散并通知治理团队。

# Telemetry & Ops

- 指标：`knowledge.delta.sla`, `knowledge.delta.approval_time`, `knowledge.feedback.loop_time`, `knowledge.feedback.fix_accuracy`, `knowledge.event.latency`, `knowledge.decay.gap_count`, `knowledge.release.gray_state`。
- 告警：增量 SLA >30m、反馈 SLA >24h、事件延迟 >5m、衰减巡检失败、灰度回滚触发。
- 可视化：Grafana `Knowledge Update Hub`、`QA Feedback Loop`、`Tenant Release Control`；报告 `reports/_state/knowledge-update.json`。

# Architecture Diagram

```mermaid
digraph G {
  rankdir=LR;
  subgraph cluster_detect {
    label="Detect & Collect";
    Scheduler -> DeltaService;
    Agent -> FeedbackSvc;
    EventBus -> EventHotfix;
  }
  DeltaService -> ApprovalCenter -> VersionStore;
  FeedbackSvc -> ReprocessPipeline -> IndexBuilder;
  EventHotfix -> HotIndex;
  VersionStore -> ReleaseController -> Tenants;
  DecayMonitor -> GapTasks -> ReprocessPipeline;
  ReleaseController -> Audit;
  Audit -> Observability;
}
```

# Open Issues & Follow-ups

| 风险/事项 | 影响范围 | 负责人 | ETA |
|-----------|----------|--------|-----|
| Usecase Seeds (`UC-KNOWLEDGE-UPDATE-FEEDBACK/EVENT/DECAY/TENANT-001`) 待创建 | Usecase 分发 | Knowledge Ops Squad | 2025-02-28 |
| 反馈/衰减指标尚未写入 `reports/_state/knowledge-update.json` | 可观测性 | Reliability Squad | 2025-02-26 |
| 事件热更新幂等策略待落地至 `event_hotfix_policies.yaml` | 实时更新 | Core Platform Squad | 2025-02-25 |
