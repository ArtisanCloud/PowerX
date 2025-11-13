---
scn_id: "SCN-KNOWLEDGE-RAG-FEEDBACK-001"
scenario_name: "RAG 反馈闭环与知识图谱协同"
slug: "knowledge-rag-feedback"
primary_scope: "powerx"
primary_layer: "service"
primary_domain: "knowledge"
primary_repo: "powerx"
doc_owner: "Kevin Zhou / Agent Experience Lead / agent-experience@artisan-cloud.com"
last_generated_at: "2025-11-12"
---

# RAG 反馈闭环与知识图谱协同 Usecase Seed 生成指南

> 场景摘要：当 Agent/用户标记“答案不准确或引用过时”时，Feedback Collector 需回溯引用链、Quality Scorer 计算质量分并触发再加工流水线，24 小时内热更新向量/倒排/图谱并完成审计回放，失败须自动回滚。

本文面向 `powerx` 仓库的 Agent Experience／Knowledge Platform 守护人，说明如何维护 `UC-KNOWLEDGE-RAG-FEEDBACK-001` Seed，以支撑 `SCN-KNOWLEDGE-RAG-FEEDBACK-001` 的交付与分发。

## Seed 的定位

- 描述 feedback-loop、quality scoring、reprocess pipeline、audit/告警在 PowerX Core 的职责矩阵。
- 与 `docs/_data/docmap.yaml` 中 `doc_id: UC-KNOWLEDGE-RAG-FEEDBACK-001` 完全对应，是 `publish:usecases` 唯一数据源。
- 指导如何把场景指标（SLA ≤24h、准确率提升 ≥30%、失败自动回滚）落地为仓库级实现。

## 前提条件

- 场景文档：`docs/scenarios/knowledge/SCN-KNOWLEDGE-RAG-FEEDBACK-001.md`；主链路：`docs/scenarios/knowledge/SCN-KNOWLEDGE-SPACE-001.md`。
- docmap：`scope: powerx`, `layer: service`, `domain: knowledge`, `path: docs/usecases-seeds/SCN-KNOWLEDGE-SPACE-001/UC-KNOWLEDGE-RAG-FEEDBACK-001.md`。
- repos：`docs/_data/repos.yaml` 中 `key: powerx` 的 `usecase_seed_root = docs/use_cases/_from_hub`、`usecase_seed_legacy_root = docs/usecases-seeds`。
- 依赖：Feature Flags `feedback.loop`, `quality.scoring`, `hot-index-update`, `graph.delta-sync`; 事件总线 `knowledge.feedback.*`; 指标脚本 `scripts/qa/workflow-metrics.mjs`; dashboard `feedback-loop`；审计模板 `docs/ops/templates/feedback-incident.md`。

> **额外依赖**：需要访问加密的反馈存储（PII 脱敏）、`scripts/ops/reprocess-runbook.mjs`（人工回滚）、`reports/_state/feedback.json`（指标导出）。

## 生成流程

1. **登记/更新 docmap 子节点**

   ```yaml
   - scn_id: SCN-KNOWLEDGE-SPACE-001
     title: 知识空间构建
     children:
       - doc_id: UC-KNOWLEDGE-RAG-FEEDBACK-001
         scope: powerx
         layer: service
         domain: knowledge
         optional: false
         repo: powerx
         path: docs/usecases-seeds/SCN-KNOWLEDGE-SPACE-001/UC-KNOWLEDGE-RAG-FEEDBACK-001.md
   ```

   - `doc_id`、`scope/layer/domain`、`path` 必须与 Seed frontmatter 一致；任何调整需同步 docmap。

2. **复制模板并放置到目录**

   ```bash
   mkdir -p docs/usecases-seeds/SCN-KNOWLEDGE-SPACE-001
   cp docs/usecases-seeds/_template.md \
     docs/usecases-seeds/SCN-KNOWLEDGE-SPACE-001/UC-KNOWLEDGE-RAG-FEEDBACK-001.md
   ```

   - 该场景使用 legacy 目录；如迁移到 `docs/use_cases/_from_hub`, 请修改 docmap + repo `usecase_seed_root`。

3. **填充 Frontmatter**

   - `owners` 至少包含 Feedback/Agent 负责人；`contributors` 可填写 Knowledge Ops、Security Ops。
   - `feature_flags`：`feedback-loop`, `hot-index-update`, `graph-delta-sync`, `quality.scoring`。
   - `code_refs`：
     - `services/feedback/collector.go` — 反馈采集/引用回溯
     - `services/feedback/quality_scorer.go` — 质量评分策略
     - `services/reprocess/pipeline.go` — 再加工任务
     - `pkg/audit/feedback_audit_logger.go` — 审计/回滚

4. **完善正文章节**

   - `Usecase Overview`：引用 SLA、准确率提升、失败回滚指标。
   - `Context & Assumptions`：说明输入（反馈 payload、chunk IDs、tool trace）、输出（reprocess 任务、热更新版本、通知）、边界（不含前端 UI）。
   - `Solution Blueprint`：拆解 Intake → Quality → Reprocess → Publish；可重用场景 mermaid（Agent → FeedbackSvc → QualitySvc → Reprocess → Audit）。
   - `Contracts & Interfaces`：列 `POST /feedback`, `GET /feedback/{id}`, `POST /feedback/{id}/resolve`, Events `knowledge.feedback.created/completed/failed`，配置 `feedback_sla_minutes`, `reprocess.max_concurrency`, `alerts.feedback_spike_threshold`。
   - `Implementation Checklist`～`Follow-ups`：涵盖反馈 schema 校验、任务编排、审计、告警、版本化、回滚脚本。

5. **互链场景文档**

   - Seed `References` 指向 `docs/scenarios/knowledge/SCN-KNOWLEDGE-RAG-FEEDBACK-001.md`、`scripts/qa/workflow-metrics.mjs`、`docs/ops/templates/feedback-incident.md`。
   - 在场景 “Usecase Links” 中链接 `UC-KNOWLEDGE-RAG-FEEDBACK-001`（可使用 Markdown `[Seed](...)`）。

## 自检清单

- [ ] docmap ↔ Seed frontmatter 一致；`optional` 与实际交付要求匹配。
- [ ] Seed 正文覆盖业务目标、上下文、体系分解、流程、契约、清单、测试、观测、回滚、风险。
- [ ] SLA/指标/告警阈值与场景描述一致（24h 闭环、>50/小时告警、回滚链路完整）。
- [ ] Feature Flag、脚本、配置路径存在并在 `repos.yaml` 定义的仓库中可访问。
- [ ] `npm run publish:usecases -- --scn-id SCN-KNOWLEDGE-SPACE-001 --validate-only` 通过。

## 常见问题

| 问题 | 处理方式 |
|------|----------|
| 反馈量激增（>50/小时） | 设置 `alerts.feedback_spike_threshold` 并在 Seed 的 Observability 中强制 PagerDuty 升级；必要时扩容再加工集群。 |
| 再加工失败无法回滚 | 记录 `scripts/ops/reprocess-runbook.mjs` 或 `POST /feedback/{id}/rollback` 的操作步骤，确保审计与告警同步。 |
| 热更新失败导致检索旧版本 | 在 `Rollback` 小节描述如何回退索引版本，并在 Seed Checklist 中加入“热更新失败报警 + 手动驱动 reprocess”。 |
| 反馈数据含 PII | 在 `Contracts` 中明确字段脱敏与加密要求，指出敏感字段只保留哈希；在 Security 章节记录审批流程。 |
| 多语言 chunk 质量评分缺算法 | 在 `Follow-ups` 中记录多语言支持风险（参见场景 open issue），并安排 AI Infra 迭代时间表。 |

完成上述工作后即可提交 PR，@knowledge-platform-stewards 评审，并在发布前执行 `publish:usecases` 验证。EOF
