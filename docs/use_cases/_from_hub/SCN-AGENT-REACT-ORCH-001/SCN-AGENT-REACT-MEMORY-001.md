---
scn_id: SCN-AGENT-REACT-MEMORY-001
title: 观察反馈与记忆更新
status: Draft
version: v0.1.0
owners:
  - name: Agent Platform Guild
    role: Observation Pipeline Owner
    contact: agent-platform@artisan-cloud.com
  - name: Knowledge Intelligence Team
    role: Memory & Knowledge Partner
    contact: knowledge@artisan-cloud.com
  - name: Ops Reliability Center
    role: Loop Governance Partner
    contact: ops-center@artisan-cloud.com
domains: [agent-orchestration]
layers: [service, ops]
repos:
  - key: powerx
    scope: core-platform
    responsibility: Observation 解析、置信度评估、循环控制、用户反馈摘要
  - key: powerx
    scope: knowledge
    responsibility: 记忆写入、知识更新触发、片段去重/水位线
related_usecases:
  - doc_id: UC-AGENT-REACT-MEMORY-001
    layer: service
    domain: agent-orchestration
last_reviewed_at: 2025-02-21
---

# Executive Summary

该子场景负责把插件返回的 Observation 转化为结构化信号，判断假设是否成立、是否需要继续循环，并决定何时把信息写入短期记忆或长期知识。目标是保持 Observation 解析准确率 ≥98%、循环步数 ≤6、在高风险或无效循环时主动中断并请求人工协同。成功信号：Observation 自动归档、置信度与引用同步到思考链、记忆写入具备幂等性、循环超阈触发告警。

# Scope & Guardrails

- **In Scope**：Observation 解析模板、置信度与引用绑定、循环控制策略、终止/人工接管逻辑、短期记忆缓存、长期知识更新触发、用户/审计反馈写回。
- **Out of Scope**：插件计算逻辑、记忆可视化 UI、知识库重建或向量训练。
- **Environment & Flags**：`react-observation-pipeline`、`react-loop-guard`、`memory-sync-service`；依赖 LLM Gateway、Knowledge Store、Audit、Notification。

# Participants & Responsibilities

| Scope | Repository | Layer | 责任与交付物 | Owners |
|-------|------------|-------|--------------|--------|
| observation-pipeline | powerx | service | 解析模板、置信度计算、引用绑定、循环终止条件 | Agent Platform Guild |
| memory-service | powerx | service | 短期记忆缓存、长期知识写入、冲突检测、TTL | Knowledge Intelligence Team |
| ops-guard | powerx | ops | 循环阈值策略、人工协同、告警/Runbook | Ops Reliability Center |

# End-to-End Flow

1. **Stage 1 – Observation Parsing**：Action 返回后通过模板或 LLM 解析器提取结论、证据、指标、错误信息，并绑定来源片段。
2. **Stage 2 – Hypothesis Evaluation**：Reasoner 根据 Observation 更新 Thought 状态，判断假设是否成立、是否需要新 Thought 或直接收敛。
3. **Stage 3 – Memory Decision**：高价值 Observation 写入短期记忆（对当前会话可复用），若满足策略或用户确认则调用知识更新管道。
4. **Stage 4 – Loop Governance & Feedback**：累计循环步数/耗时；若超阈或失败率过高，触发中断、人工接管和告警；同时向用户输出阶段性总结。

# Architecture Diagram

```mermaid
sequenceDiagram
  participant Actor as 行动 Agent
  participant Parser as Observation Pipeline
  participant Reasoner as 主 Agent
  participant Memory as 记忆/知识服务
  participant Ops as 风控/告警

  Actor-->>Parser: Observation Payload
  Parser-->>Reasoner: 结构化 Observation + 置信度
  Reasoner->>Memory: 写入短期记忆/知识更新
  Memory-->>Reasoner: 写入结果/冲突提示
  Reasoner->>Ops: 循环指标、告警状态
  Ops-->>Reasoner: 中断/人工接管决策
```

# Key Interactions & Contracts

- **APIs / Events**
  - `POST /internal/react/observation`：Body 含 `action_id`, `payload`, `schema_version`; 返回解析结果、置信度、引用。
  - `POST /internal/react/memory`：写入 `type=[short_term|long_term]`, `content`, `source_refs`, `expires_at`。
  - `EVENT react.loop.state`：广播循环步数、耗时、阈值状态、人工接管原因。
- **Configs / Schemas**
  - `schemas/react_observation.json`、`config/react/loop_guard.yaml`、`config/memory/policy.yaml`。
  - `docs/standards/powerx/backend/integration/09_agent/Agent_Manager_and_Lifecycle_Spec.md`。
- **Security / Compliance**
  - 记忆写入遵循租户隔离、敏感字段脱敏，长记忆需审计与可撤销。
  - 循环中断需记录原因、操作人、原始 Trace。

# Usecase Links

- `UC-AGENT-REACT-MEMORY-001` — Observation 解析与记忆治理（service 层，`docs/usecases-seeds/SCN-AGENT-REACT-ORCH-001/UC-AGENT-REACT-MEMORY-001.md`）。

# Acceptance Criteria

1. Observation 解析准确率 ≥98%，所有字段均提供引用 ID 与置信度。
2. ReAct 循环步数 ≤6、耗时 ≤45 秒；超阈值立即中断并触发人工协同。
3. 记忆写入幂等，重复 Observation 不会污染知识库，并提供回滚句柄。
4. 阶段性总结向用户输出时必须包含引用来源与置信度提示。

# Telemetry & Ops

- **指标**：`react.observation.parse_success_rate`、`react.loop.steps_total`、`react.loop.break_count`、`react.memory.write_success_rate`、`react.memory.conflict_total`。
- **日志/审计**：`audit.react_observation` 记录 action_id、引用、置信度；`audit.react_memory` 记录写入类型、TTL、触发策略。
- **告警**：Observation 解析失败率 >2%、循环超阈事件 >0、记忆冲突 >5 次/小时；通知 Ops 值班与 Teams #agent-react。
- **Tools**：`scripts/ops/react-loop-drill.mjs`、`node scripts/qa/workflow-metrics.mjs --metric react.loop.steps_total`。

# Open Issues & Follow-ups

| 风险/事项 | 影响范围 | 负责人 | ETA |
|-----------|----------|--------|-----|
| 长记忆写入缺乏审批/撤销接口 | 知识污染、审计不可控 | Knowledge Intelligence Team | 2025-03-06 |
| 循环中断后的用户体验方案待设计 | 人工接管体验不一致 | Agent Platform Guild | 2025-03-10 |

# Appendix

- `docs/meta/scenarios/powerx/agent-and-automation/agent-orchestration/react-agent-orchestration/primary.md`
- `docs/_data/docmap.yaml`
