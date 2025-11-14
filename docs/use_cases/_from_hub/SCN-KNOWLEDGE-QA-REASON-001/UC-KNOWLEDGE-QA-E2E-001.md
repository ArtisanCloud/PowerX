---
scn_id: "SCN-KNOWLEDGE-QA-REASON-001"
scenario_name: "智能问答与推理"
slug: "intelligent-qa-reasoning"
primary_scope: "powerx"
primary_layer: "service"
primary_domain: "knowledge"
primary_repo: "powerx"
doc_owner: "Michael Hu / Product Manager / matrix-x@artisan-cloud.com"
last_generated_at: "2025-02-14"
---

# 智能问答与推理 Usecase Seed 生成指南

> 场景摘要：该场景覆盖“终端提问 → 多知识空间检索 → 工具推理 → 引用/审计 → 反馈闭环”的整条链路，要求 2 秒内返回跨空间引用、推理链完整可回放、实时工具成功率 ≥99%、越权拦截率 100%、负向反馈 24 小时闭环。

本文档面向 `powerx` 仓库的 QA Orchestrator、智能体编排与安全合规 Stewards，指导如何维护 `UC-KNOWLEDGE-QA-E2E-001` 种子数据并交付 `SCN-KNOWLEDGE-QA-REASON-001` 主流程。

## Seed 的定位

- 描述 PowerX Core 在智能问答主链路中承担的端到端职责：意图接入、跨空间检索、工具推理、引用输出、审计与反馈闭环。
- 与 `docs/_data/docmap.yaml` 中 `scn_id: SCN-KNOWLEDGE-QA-REASON-001` → `doc_id: UC-KNOWLEDGE-QA-E2E-001` 完全一致，是 `publish:usecases` / `publish:collected` 的唯一权威来源。
- 统筹后续子场景（Retrieval / Context / Tool / Feedback / Compliance）的共同指标、Feature Flag 与跨仓契约。

## 前提条件

- 场景文档：`docs/scenarios/knowledge/SCN-KNOWLEDGE-QA-REASON-001.md`。
- docmap：`scope: powerx`, `layer: application`, `domain: knowledge`, `path: docs/usecases-seeds/SCN-KNOWLEDGE-QA-REASON-001/UC-KNOWLEDGE-QA-E2E-001.md`。
- repos：`docs/_data/repos.yaml` → `key: powerx`，`usecase_seed_root = docs/use_cases/_from_hub`，`usecase_seed_legacy_root = docs/usecases-seeds`。
- Feature Flags：`PX_QA_ORCHESTRATOR`, `PX_AGENT_MEMORY`, `PX_QA_MULTI_SPACE`, `PX_QA_TOOLCHAIN`, `PX_SECURITY_AUDIT_LOG`, `PX_QA_FEEDBACK_LOOP`。
- 依赖服务：`knowledge-space`（多空间检索）、`tool-runtime`（SQL/REST/规则）、`agent-dialogue`（上下文记忆）、`audit-ledger`、`feedback-service`。
- 运行脚本：`scripts/qa/workflow-metrics.mjs`, `scripts/dev/seed_intelligent_qa.mjs`, `reports/_state/workflows/SCN-KNOWLEDGE-QA-REASON-001.json`。

> **若需要额外凭据/数据源**（例如 BI 仓库、敏感字段词典），请在此列表继续补充并抄送安全团队审批。

## 生成流程

1. **登记/更新 docmap 子节点**

   ```yaml
   - scn_id: SCN-KNOWLEDGE-QA-REASON-001
     title: 智能问答与推理
     children:
       - doc_id: UC-KNOWLEDGE-QA-E2E-001
         scope: powerx
         layer: application
         domain: knowledge
         optional: false
         repo: powerx
         path: docs/usecases-seeds/SCN-KNOWLEDGE-QA-REASON-001/UC-KNOWLEDGE-QA-E2E-001.md
   ```

   - 如需扩展新的仓库/子流程（例如第三方 Agent 控制台），先更新 docmap 再补 Seed，保持 `scope/layer/domain` 一致。

2. **复制模板并放置到场景目录**

   ```bash
   mkdir -p docs/usecases-seeds/SCN-KNOWLEDGE-QA-REASON-001
   cp docs/usecases-seeds/_template.md \
     docs/usecases-seeds/SCN-KNOWLEDGE-QA-REASON-001/UC-KNOWLEDGE-QA-E2E-001.md
   ```

   - 若将来迁移到 `docs/use_cases/_from_hub`，需同步更新 repo 配置与 docmap `path`。

3. **填充 Frontmatter 区域**

   - `owners` 至少包含 Product（Michael Hu）、Agent Experience、Security & Compliance 代表；`contributors` 可列出 Tool Runtime / Knowledge Ops。
   - `feature_flags` 建议列出上文 Flag，并指向 `configs/feature_flags.yaml` 的落盘位置。
   - `code_refs`：如 `services/qa/orchestrator/service.go`, `services/agent/dialogue/context_store.go`, `services/security/compliance/audit_logger.go`, `services/feedback/processor.go`。

4. **完善正文章节**

   - `Usecase Overview`：重申响应 ≤2s、引用覆盖 ≥95%、工具成功率 ≥99%、越权拦截率 100%、反馈 24h 闭环等指标。
   - `Context & Assumptions`：列出租户前置、知识空间准备、Agent 认证、工具凭据、审计数据湖权限等边界。
   - `Solution Blueprint`：沿用场景四个 Stage（Intent Intake / Multi-space Retrieval / Tool-chained Reasoning / Answer & Feedback），可追加 mermaid 时序针对具体代码模块。
   - `Contracts & Interfaces`：详细说明 `POST /qa/intents/analyze`, `POST /qa/retrieval/multi-space`, `POST /qa/answers/compose`, `POST /tools/sql/run`, `POST /audit/logs`, `POST /feedback/events` 的请求/响应与幂等策略。
   - `Implementation Checklist` ～ `Observability`：覆盖 Feature Flag、配置、测试脚本（含逆向/容错）、Grafana 仪表、告警阈值、reports/_state 产物。
   - `Rollback & Failure Handling`：描述检索降级、工具失败回退、越权拦截、反馈积压、SLA 超时的操作手册。

5. **与场景文档互链**

   - 在该 Seed “References” 段落中引用主场景 `docs/scenarios/knowledge/SCN-KNOWLEDGE-QA-REASON-001.md`、相关 meta 文档及标准。
   - 在场景 “Usecase Links” 中保持 `[UC-KNOWLEDGE-QA-E2E-001](docs/usecases-seeds/SCN-KNOWLEDGE-QA-REASON-001/UC-KNOWLEDGE-QA-E2E-001.md)` 链接可用。

## 自检清单

- [ ] docmap 与 Seed frontmatter 完全一致（`doc_id/scope/layer/domain/path/optional`）。
- [ ] Seed 正文覆盖业务目标、上下文、流程分解、接口契约、测试矩阵、运维与回滚策略。
- [ ] 指标/告警与场景文档一致：跨空间检索 p95 ≤2s、引用覆盖 ≥95%、工具成功率 ≥99%、越权拦截率 100%、反馈闭环 ≤24h。
- [ ] Feature Flag 与配置文件存在并在 `dev/staging/prod` 环境同步。
- [ ] `npm run lint`、`npm run docs:build`、`npm run publish:usecases -- --scn-id SCN-KNOWLEDGE-QA-REASON-001 --validate-only` 均通过。
- [ ] 生成 `reports/_state/workflows/SCN-KNOWLEDGE-QA-REASON-001.json` 并附加在 PR 说明。

## 常见问题

| 问题 | 处理方式 |
|------|----------|
| 缺少跨空间引用或降级提示 | 检查 `retrieval_strategy.json`、`audit.retrieval.degrade_reason`；保证降级消息落入回答与审计。 |
| 工具失败导致回答超时 | 使用 `PX_QA_TOOLCHAIN` failover 策略：回退缓存数据或提示“稍后重试”，并写入 `qa.failover.count` 指标。 |
| 越权/敏感审计缺失 | 确保调用 `/security/access/check`、`/security/sensitive/detect`，并把 `audit.reasoning_steps` 与 `audit.answer_id` 绑定。 |
| 反馈闭环 SLA 超时 | 触发 `feedback.high_priority` 告警并自动升级工单，参考 Feedback 子场景 Seed 的处理脚本。 |

完成以上步骤后，可提交 PR 并通知 `@qa-orchestrator-stewards` 与 `@agent-experience-stewards` 评审，随后执行 `npm run publish:usecases -- --scn-id SCN-KNOWLEDGE-QA-REASON-001` 将 Seed 同步到下游仓库。
