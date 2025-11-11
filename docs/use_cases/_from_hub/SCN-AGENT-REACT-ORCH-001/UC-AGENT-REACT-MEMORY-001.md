doc_id: UC-AGENT-REACT-MEMORY-001
scn_id: SCN-AGENT-REACT-ORCH-001
title: PowerX (service) - ReAct 观察反馈与记忆治理
status: Draft
version: v0.1.0
repo_key: powerx
scope: powerx
layer: service
domain: agent-orchestration
scenario_title: "ReAct 智能体编排"
owners:
  - name: Agent Platform Guild
    role: Observation Pipeline Owner
    contact: agent-platform@artisan-cloud.com
contributors:
  - name: Knowledge Intelligence Team
    role: Memory & Knowledge Partner
    contact: knowledge@artisan-cloud.com
  - name: Ops Reliability Center
    role: Loop Governance Partner
    contact: ops-center@artisan-cloud.com
linked_requirements:
  - id: SCN-AGENT-REACT-ORCH-001
    description: 主场景 - ReAct 智能体编排
  - id: SCN-AGENT-REACT-MEMORY-001
    description: 子场景 C - 观察反馈与记忆更新
code_refs:
  - repo: powerx
    path: services/react/observation-parser.ts
    description: Observation 解析、摘要、引用与置信度计算
  - repo: powerx
    path: services/memory/short_term.ts
    description: 会话级记忆缓存与 TTL 管理
  - repo: powerx
    path: services/memory/long_term.ts
    description: 高价值 Observation 写入长期知识 / 知识更新触发
  - repo: powerx
    path: services/observability/react_loop_metrics.ts
    description: ReAct 循环步数、超时、Breakdown 指标
  - repo: powerx
    path: config/react/loop_guard.yaml
    description: 循环阈值、风控策略、人工协同配置
feature_flags:
  - react-observation-pipeline
  - react-loop-guard
  - memory-sync-service
last_reviewed_at: 2025-02-21

---

# Usecase Overview

- **业务目标**：在每次 Action 完成后解析 Observation、评估置信度与假设状态，并根据策略写入短期/长期记忆，同时监控 ReAct 循环步数、耗时与异常。
- **成功度量**：Observation 解析准确率 ≥98%；循环步数 ≤6、`react.loop.break_count` 为 0；记忆写入冲突率 <0.5%；阶段性总结包含引用且置信度>=0.7；超阈值中断触发率 <2%。
- **场景关联**：承接 Action 输出，为 Audit/Closure 提供结构化 Observation 与记忆；与 `SCN-AGENT-REACT-AUDIT-001` 联动生成回放。

> 摘要：该用例定义“Observation 解析 → 假设评估 → 记忆写入 → 循环治理”全链路，确保 ReAct 循环稳定、可控，可向下游输出可信记忆。

# Context & Assumptions

- **前置条件**
  - Feature Flags `react-observation-pipeline`, `react-loop-guard`, `memory-sync-service` 已启用。
  - 知识/记忆存储（Redis Short-term、Vector Long-term、Audit DB）可访问。
  - `config/react/loop_guard.yaml` 定义了循环步数、超时、敏感操作阈值与人工协同条件。
- **输入/输出**
  - 输入：`action_id`, `observation_payload`, `metrics`, `error?`, `trace_id`, `risk_level`, `tenant_ctx`。
  - 输出：结构化 `observation`, `confidence`, `source_refs`, `hypothesis_state`, `loop_state`, `memory_record?`, `break_reason?`, 用户阶段性总结。
- **边界**
  - 不负责行动执行或审批。
  - 不生成最终回放报告（Audit 用例负责）。
  - 不处理知识库重建或大规模索引刷新，仅触发写入流程。

# Solution Blueprint

## 体系分解

| 层 | 主要组件/模块 | 责任 | 代码入口 |
|----|---------------|------|---------|
| service | `services/react/observation-parser.ts` | 解析 Action 输出、生成结构化 Observation、置信度、引用 | `services/react/observation-parser.ts` |
| service | `services/memory/short_term.ts` | 会话级记忆缓存、幂等写入、TTL & eviction | `services/memory/short_term.ts` |
| service | `services/memory/long_term.ts` | 高价值 Observation 写入知识库、冲突检测、审批 | `services/memory/long_term.ts` |
| ops | `services/observability/react_loop_metrics.ts` | 步数、耗时、Breakdown 指标；触发 `react.loop.state` 事件 | `services/observability/react_loop_metrics.ts` |
| ops | `config/react/loop_guard.yaml` | 循环阈值、人工协同条件、异常告警配置 | `config/react/loop_guard.yaml` |

## 流程与时序

1. **Step 1 – Observation Parsing**：接收 Action 响应，使用模板/LLM 解析器提取结论、证据、指标、错误，并附带 `source_refs`、`confidence`。
2. **Step 2 – Hypothesis Evaluation**：更新 Thought 状态，决定是否生成新的 Thought/Action 或收敛；同步 `loop_state` 指标。
3. **Step 3 – Memory Decision**：根据策略（置信度、权重、用户确认）决定写入短期/长期记忆；检测重复/冲突，必要时触发审批或人工。
4. **Step 4 – Loop Governance**：累积循环步数、耗时、失败率；若超阈或失败率过高，触发中断或人工接管，同时向用户输出阶段性总结。
5. **Step 5 – Broadcast & Audit**：发送 `react.loop.state` 事件、写入 Audit/Telemetry，并把记忆引用交给 Audit 回放。

```mermaid
sequenceDiagram
  participant Actor as 行动执行器
  participant Parser as Observation Pipeline
  participant Loop as Loop Guard
  participant Memory as Memory Service
  participant Audit as Audit/Telemetry

  Actor-->>Parser: observationPayload + trace_id
  Parser->>Loop: parsedObservation + metrics
  alt 可写记忆
    Loop->>Memory: writeShortTerm(observation)
    Memory-->>Loop: ack / conflict
    opt 高价值
      Loop->>Memory: requestLongTermWrite()
    end
  end
  Loop-->>Audit: emit react.loop.state + summary
  Loop-->>Reasoner: observation + loop decision
```

# Contracts & Interfaces

- **Inbound APIs / Events**
  - `POST /internal/react/observation` — Body: `action_id`, `payload`, `schema_version`, `metrics`; 返回 `observation`, `confidence`, `hypothesis_state`.
  - `POST /internal/react/loop_guard` — 更新循环状态、步数、阈值。
- **Outbound 调用**
  - `POST /internal/react/memory` — `type=short_term|long_term`, `content`, `source_refs`, `trace_id`, `ttl`, `approver?`。
  - `EVENT react.loop.state` — 包含 `trace_id`, `step`, `decision`, `reason`, `confidence`, `memory_ref`, `break?`。
- **配置与脚本**
  - `config/react/loop_guard.yaml`, `config/memory/policy.yaml`。
  - `scripts/ops/react-loop-drill.mjs`, `scripts/ops/memory-sync.mjs`。

# Implementation Checklist

| 项目 | 描述 | 完成状态 | 负责人 |
|------|------|----------|--------|
| Observation Parser | 模板、LLM、引用提取、错误分类 | [ ] | Agent Platform Guild |
| Loop Guard | 阈值管理、Break 策略、人工协同接口 | [ ] | Ops Reliability Center |
| Memory Service | 短期缓存、长期写入、审批/撤销、冲突检测 | [ ] | Knowledge Intelligence Team |
| Telemetry & Audit | `react.loop.*`, `react.memory.*` 指标、Audit Schema | [ ] | Ops Reliability Center |
| CLI/Runbooks | `react-loop-drill`, `memory-sync`、Break Runbook | [ ] | Agent Platform Guild |

# Testing Strategy

- **单元**：Observation 解析器（正确性、异常）、记忆幂等性、冲突检测、Loop 阈值逻辑。
- **集成**：Mock Memory/Audit，验证写入、审批、冲突处理；模拟循环超阈、中断、人工协同。
- **端到端**：运行 `scripts/ops/react-loop-drill.mjs --tenant tenant-react-lab --max-steps 6`，模拟成功/失败循环，验证指标与告警。
- **非功能**：高频 Observation 写入压力测试；Chaos（Memory 服务不可用、Audit 延迟）与缓冲/重放策略。

# Observability & Ops

- **指标**：`react.observation.parse_success_rate`, `react.loop.steps_total`, `react.loop.break_count`, `react.memory.write_success_rate`, `react.memory.conflict_total`, `react.memory.long_term_pending_total`。
- **日志**：`audit.react_observation`（action_id, trace_id, confidence, references），`audit.react_memory`（type, ttl, approver, conflict_flag），Loop 警告日志记录 break 原因。
- **告警**：解析失败率 >2%、循环超阈 >0、记忆冲突 >5 次/小时、长记忆审批超时 >30 分钟；通知 PagerDuty + Teams #agent-react-loop。
- **Dashboards**：Grafana「ReAct Loop」+ 「Memory Governance」，Datadog `react.loop.*`, `react.memory.*`，Workflow 报表。

# Rollback & Failure Handling

- **回滚步骤**：关闭 `react-observation-pipeline` Feature Flag、回滚 Parser/Mem 服务、还原 Loop 阈值。
- **补救措施**：Observation 解析失败→切换到规则模板；Memory 写入失败→缓存重试 + 人工审批；循环超阈→自动降级或人工接管；记忆冲突→触发冲突解决 UI。
- **数据修复**：`scripts/ops/react-observation-replay.mjs --trace <id>` 补写审计；`memory-admin revoke --record <id>` 回滚错误记忆。

# Follow-ups & Risks

| 风险/事项 | 影响 | 缓解方案 | 负责人 | ETA |
|-----------|------|----------|--------|-----|
| 记忆写入缺乏审批/撤销 | 污染知识库、审计不可控 | 引入审批/撤销接口、记录引用来源 | Knowledge Intelligence Team | 2025-03-06 |
| 循环中断用户体验不足 | 人工接管效率低 | 设计 UX 提示、Copilot 协同、回放链接 | Agent Platform Guild | 2025-03-10 |
| 长期存储成本上升 | 记忆滞留、查询慢 | TTL、冷热分层、压缩策略 | Ops Reliability Center | 2025-03-18 |

# References & Links

- 场景：`docs/scenarios/agent-orchestration/SCN-AGENT-REACT-MEMORY-001.md`
- 主场景：`docs/scenarios/agent-orchestration/SCN-AGENT-REACT-ORCH-001.md`
- 标准：`docs/standards/powerx/backend/integration/09_agent/Agent_Manager_and_Lifecycle_Spec.md`
- Runbook：`runbooks/agent/react_loop_break.md`
- QA：`scripts/ops/react-loop-drill.mjs`, `scripts/ops/memory-sync.mjs`
