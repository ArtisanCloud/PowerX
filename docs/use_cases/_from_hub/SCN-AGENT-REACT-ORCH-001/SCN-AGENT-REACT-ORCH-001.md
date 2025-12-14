---
scn_id: SCN-AGENT-REACT-ORCH-001
title: ReAct 智能体编排
status: Draft
version: v0.1.0
owners:
  - name: Agent Platform Guild
    role: ReAct Orchestration Steward
    contact: agent-platform@artisan-cloud.com
  - name: Knowledge Intelligence Team
    role: Retrieval & Memory Partner
    contact: knowledge@artisan-cloud.com
  - name: Ops Reliability Center
    role: Risk & Observability Partner
    contact: ops-center@artisan-cloud.com
domains: [agent-orchestration]
layers: [service, integration, ops]
repos:
  - key: powerx
    scope: core-platform
    responsibility: ReAct 编排引擎、Chain-of-Thought 持久化、Action Router、策略钩子
  - key: powerx
    scope: knowledge
    responsibility: 多策略知识检索、记忆写入、片段评分
  - key: powerx
    scope: ops
    responsibility: 风险控制、审批流、回放与审计可视化
  - key: powerx-plugin
    scope: plugin-ecosystem
    responsibility: 插件目录、执行器 SDK、调用追踪与风险标注
related_usecases:
  - doc_id: UC-AGENT-REACT-THOUGHT-001
    layer: service
    domain: agent-orchestration
  - doc_id: UC-AGENT-REACT-ACTION-001
    layer: integration
    domain: agent-orchestration
  - doc_id: UC-AGENT-REACT-MEMORY-001
    layer: service
    domain: agent-orchestration
  - doc_id: UC-AGENT-REACT-AUDIT-001
    layer: ops
    domain: agent-orchestration
last_reviewed_at: 2025-02-21
---

# Executive Summary

ReAct 智能体编排让主 Agent 能在一次对话中完成“思考 → 检索 → 行动 → 观察 → 反馈”闭环，并对每一步进行审计、风险约束与可视化。该场景聚焦链路级治理：把知识空间、插件生态、风险控制与人工协同统一到同一条推理轨上，避免思考链丢失、工具调用失控或敏感操作越权。成功信号包括：首个 Thought 在 1.5 秒内生成、首轮检索命中率 ≥80%、行动成功率 ≥95%、循环次数 ≤6、回放生成成功率 100%，并与“知识空间构建”“智能问答与推理”场景协同，确保答案可解释、可回放、可治理。

# Positioning & Goals

- 将 ReAct 编排确立为 Agent 层的默认执行策略，所有思考链、知识引用与工具调用都走统一的状态机与审计面板。
- 通过多策略检索和动态插件选择，让复杂问题的解答时间缩短 30%，推理置信度可量化且可追溯。
- 在每一次行动前挂钩风控策略/人工审批，敏感操作全部有迹可循并可快速回滚。
- 建立统一的观测面：指标、事件、日志与回放共享追踪 ID，支撑 QA、合规与性能调优。

# Core Capabilities

| 能力域 | 说明 | 关键系统/材料 |
|--------|------|---------------|
| Thought Lifecycle Management | 意图解析、Thought/Action 模板、循环控制与状态机守护，保证首个 Thought 在 1.5 秒内输出并记录上下文 | `services/react/thought-manager.ts`、`react-session-store`、`config/react/thought_templates.yaml` |
| Multi-Strategy Knowledge Orchestration | 向量/关键词/图谱混合检索、片段打分、引用上下文裁剪与敏感字段脱敏 | `services/knowledge/search.ts`、`knowledge-hub`、`config/knowledge/routing.yaml` |
| Action Governance & Tool Routing | 自动或人工批准插件调用、参数模板、风控钩子、失败降级、人工协同 | `services/react/action-router.ts`、`plugin-catalog`、`workflow/agent_action_guard.yaml` |
| Observation & Memory Fabric | Observation 摘要、置信度评估、短期记忆缓存、长期知识更新 | `services/memory/short_term.ts`、`services/memory/long_term.ts`、`scripts/ops/memory-sync.mjs` |
| Audit & Feedback Loop | 回放轨迹、指标打点、用户/审计反馈回写策略权重 | `services/observability/react_audit.ts`、`workflow-metrics.mjs`、`reports/react/**` |

# Scope & Guardrails

- **In Scope**：ReAct 会话生命周期、思考链建模、检索策略路由、插件调用治理、Observation 解析与记忆写入、回放与审计、风控/审批挂钩、指标与告警。
- **Out of Scope**：Agent 注册/授权、知识库构建流水线、LLM 模型接入、通用任务型工作流画布、Marketplace 计费，不在本主场景讨论。
- **Environment & Flags**：`react-orchestrator-v1`、`knowledge-hub-mix-search`、`agent-action-approval`、`react-loop-guard`、`react-audit-timeline`；依赖 LLM Gateway、Knowledge Store、Plugin Registry、Workflow Engine、Audit/Telemetry 总线。

# Participants & Responsibilities

| Scope | Repository | Layer | 责任与交付物 | Owners |
|-------|------------|-------|--------------|--------|
| react-orchestrator | powerx | service | 意图解析、Thought/Action 状态机、循环控制、上下文持久化、Trace 绑定 | Agent Platform Guild |
| knowledge-hub | powerx | integration | 多策略检索、片段裁剪、相似度评估、知识引用安全策略 | Knowledge Intelligence Team |
| risk-ops | powerx | ops | 风险策略、审批工作流、循环阈值与敏感操作治理、人工接管 | Ops Reliability Center |
| plugin-connectors | powerx-plugin | integration | 插件目录、权限/版本校验、执行器 SDK、调用追踪与降级策略 | Plugin Guild |

# End-to-End Flow

1. **Stage 1 – Request Intake & Intent Framing**：用户提交复杂问题后，主 Agent 生成会话 ID、追踪 ID，并用意图分类模型产生首个 Thought 与缺失信息列表。
2. **Stage 2 – Knowledge Strategy Selection**：根据任务类型、租户策略和可用信号，触发向量/关键词/图谱或组合检索，过滤低置信度片段并写入审计。
3. **Stage 3 – Action Planning & Risk Gating**：Thought 生成 Action，Action Router 结合插件能力、权限与风险标签准备调用，必要时通过审批策略或人工确认。
4. **Stage 4 – Observation & Memory Update**：每次调用的 Observation 被解析为结构化对象，补充置信度、指标、引用片段，并写入短期记忆；若满足策略则触发长期记忆或知识更新。
5. **Stage 5 – Closure & Playback Ready**：循环在满足目标或触发阈值时结束，生成最终回答、指标摘要、回放轨迹并推送给用户/审计员，反馈信息回写策略权重。

# Architecture Diagram

```mermaid
sequenceDiagram
  participant User as 业务用户
  participant Reasoner as 主 Agent (Reasoner)
  participant Knowledge as 知识空间服务
  participant Risk as 风控/审批
  participant Actor as 行动 Agent / 插件
  participant Audit as 审计与回放

  User->>Reasoner: 提交复杂问题
  Reasoner->>Knowledge: Thought #1 请求多策略检索
  Knowledge-->>Reasoner: 片段 + 相似度
  Reasoner->>Risk: Action #n 风险评估/审批
  Risk-->>Reasoner: 批准/驳回 + 限制
  Reasoner->>Actor: 调用插件/工具
  Actor-->>Reasoner: Observation #n + 指标
  Reasoner->>Audit: 写入 Trace、思考链、引用片段
  Audit-->>User: 回放/最终回答/告警
```

# Key Interactions & Contracts

- **APIs / Events**
  - `POST /internal/react/session`：创建会话与追踪 ID，Body 包含 `tenant_uuid`, `question`, `risk_profile`。
  - `POST /internal/knowledge/search`：支持 `mode=[vector|keyword|graph|hybrid]`，返回片段、评分、引用 ID。
  - `POST /internal/react/action` 与 `POST /internal/react/action/{id}/approve`：Action Router 提交工具调用与风控审批。
  - `EVENT react.chain.state.changed`：广播 Thought/Action/Observation 状态，供 Telemetry/Audit 订阅。
- **Configs / Schemas**
  - `config/react/thought_templates.yaml`、`config/react/action_policy.yaml`、`config/knowledge/routing.yaml`。
  - `docs/standards/scenarios/_template.md`、`docs/standards/powerx/backend/integration/09_agent/Agent_Manager_and_Lifecycle_Spec.md`。
- **Security / Compliance**
  - 全链 Trace ID + 零信任密钥，插件调用需携带租户上下文与签名。
  - 敏感操作分级审批、循环次数/超时硬限制、知识引用脱敏与水印。

# Usecase Links

- `UC-AGENT-REACT-THOUGHT-001` — 思考链生成与检索策略（service 层，`docs/usecases-seeds/SCN-AGENT-REACT-ORCH-001/UC-AGENT-REACT-THOUGHT-001.md`）。
- `UC-AGENT-REACT-ACTION-001` — 行动计划与插件调用（integration 层，`docs/usecases-seeds/SCN-AGENT-REACT-ORCH-001/UC-AGENT-REACT-ACTION-001.md`）。
- `UC-AGENT-REACT-MEMORY-001` — Observation & 记忆写回（service 层，`docs/usecases-seeds/SCN-AGENT-REACT-ORCH-001/UC-AGENT-REACT-MEMORY-001.md`）。
- `UC-AGENT-REACT-AUDIT-001` — 回放与审计闭环（ops 层，`docs/usecases-seeds/SCN-AGENT-REACT-ORCH-001/UC-AGENT-REACT-AUDIT-001.md`）。

# Acceptance Criteria

1. 首个 Thought 在 1.5 秒内生成且携带缺失信息/风险标注，失败会提供告警与兜底话术。
2. 首轮混合检索命中率 ≥80%，未命中会提示补充上下文或切换策略。
3. 行动执行成功率 ≥95%，敏感操作 100% 经过审批或人工确认，连续失败自动降级或终止。
4. ReAct 循环步数 ≤6、耗时 ≤45 秒，超阈值将中断并推送人工协同。
5. 任务结束后 30 秒内生成回放，包含 Thought/Action/Observation、引用片段与审批记录。

# Telemetry & Ops

- **指标**：`react.thought.latency_ms`、`react.knowledge.hit_rate`、`react.action.success_rate`、`react.loop.steps_total`、`react.loop.break_count`、`react.audit.playback_latency_ms`。
- **日志 & 审计**：每个 Thought/Action/Observation 均写入 Audit Service（含 tenant、trace_id、插件、审批结果），并在日志中脱敏输出输入/输出摘要。
- **告警**：检索命中率 <70%、Action 连续失败 ≥3、循环步数 >6、审批超时 >2 分钟、回放生成失败；通知 PagerDuty + Teams #agent-react + Ops 邮件。
- **工具**：`npm run publish:scenarios -- --scn-id SCN-AGENT-REACT-ORCH-001 --validate-only`、`node scripts/qa/workflow-metrics.mjs --scenario react-orchestrator`、`scripts/ops/react-loop-drill.mjs`。

# Validation Workflow

1. **结构校验**：执行 `npm run lint` 与 `npm run docs:build`，确保 Markdown、Mermaid、Frontmatter 与站点构建通过。
2. **Scenario 发布验证**：运行 `npm run publish:scenarios -- --scn-id SCN-AGENT-REACT-ORCH-001 --validate-only` 校验结构、docmap、指纹以及子场景引用；必要时携带 `--resume-token` 重新执行。
3. **Usecase 对齐**：针对四个 usecase 种子执行 `npm run publish:usecases -- --scn-id SCN-AGENT-REACT-ORCH-001 --validate-only`，确认种子结构、docmap children 与 scenario 匹配。
4. **工作流指标验收**：运行 `node scripts/qa/workflow-metrics.mjs --scenario react-orchestrator`、`scripts/ops/react-loop-drill.mjs` 等工具，收集 Thought/Action/Loop/Audit 指标并附在验收报告。

# Open Issues & Follow-ups

| 风险/事项 | 影响范围 | 负责人 | ETA |
|-----------|----------|--------|-----|
| 审批路径与租户风控策略版本尚未统一 | 高风险插件调用审批可能延迟或冲突 | Ops Reliability Center | 2025-03-05 |
| 记忆写回与知识更新冲突检测待实现 | 记忆/知识重复写入导致污染 | Knowledge Intelligence Team | 2025-03-12 |
| 回放存储扩容与冷热分层方案未落地 | 回放查询延迟、审计合规风险 | Agent Platform Guild | 2025-03-20 |

# Related Links

- `docs/meta/scenarios/powerx/agent-and-automation/agent-orchestration/react-agent-orchestration/primary.md`
- `docs/meta/scenarios/powerx/list.md`
- `docs/scenarios/agent-orchestration/SCN-AGENT-TASK-EXEC-001.md`
- `docs/standards/powerx/backend/integration/09_agent/Agent_Manager_and_Lifecycle_Spec.md`
- `reports/usecases/usecases_SCN-AGENT-REACT-ORCH-001.json`

# Appendix

- `docs/meta/scenarios/powerx/agent-and-automation/agent-orchestration/react-agent-orchestration/primary.md`
- `docs/_data/docmap.yaml`
- `reports/usecases/usecases_SCN-AGENT-REG-MGMT-001.json`（对齐现有 Agent 场景指标结构）
