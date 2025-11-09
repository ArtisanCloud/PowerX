doc_id: UC-AGENT-EXEC-COORD-001
scn_id: SCN-AGENT-TASK-EXEC-001
title: 多 Agent 并行执行与状态协调
status: Draft
version: v0.1.0
repo_key: powerx
scope: powerx
layer: integration
domain: agent-orchestration
scenario_title: "智能体任务执行"
owners:
  - name: Agent Platform Guild
    role: Scenario Steward
    contact: agent-platform@artisan-cloud.com
  - name: Ops Reliability Center
    role: Automation Co-owner
    contact: ops-center@artisan-cloud.com
contributors: []
linked_requirements:
  - SCN-AGENT-TASK-EXEC-001-B
code_refs:
  - repo: powerx
    path: services/agent/orchestrator/dag_runtime.ts
    description: 任务 DAG 运行器、依赖与优先级执行
  - repo: powerx
    path: services/agent/subagents/dispatcher.ts
    description: 子 Agent 任务领取、上下文装载
  - repo: powerx
    path: services/statebus/event_streamer.ts
    description: 状态总线上报与订阅
  - repo: powerx
    path: services/agent/controller/rebalance_manager.ts
    description: 调度策略调整、重排与副本扩缩
feature_flags:
  - agent-orchestrator-v2
  - statebus-stream
  - scheduler-autoscale
optional: false
last_reviewed_at: 2025-02-15

---

# Usecase Overview

- **业务目标**：让主 Agent 将任务 DAG 拆分给多个子 Agent 并行执行，实时汇报状态与部分结果，确保高吞吐、低延迟并支持动态调度。
- **成功度量**：并行子任务成功率 ≥95%；状态同步延迟 <1 秒；阻塞任务可在 SLA 内被检测并处理；结果汇总可追踪。
- **场景关联**：承接 Stage 2「Parallel Execution & State Coordination」，为后续失败恢复、闭环校验提供实时状态与上下文。

> 关键在于 DAG 与状态总线的联动，确保调度器能依据实时信号进行重排、扩缩和限流。

# Context & Assumptions

- **前置条件**
  - 状态总线（Kafka/SQS 或等价实现）可用，且已启用 `statebus-stream` Flag。
  - 子 Agent 注册表含有租户、权限、工具清单与限流策略。
  - 插件调用配额、凭证和追踪 ID 已注入。
- **输入/输出**
  - 输入：Planner 产出的任务 DAG、租户上下文、子 Agent 池、执行策略（优先级、并行度、限流、幂等键）。
  - 输出：状态事件流、阶段结果、上下文快照、阻塞告警、最终合并的任务交付物。
- **边界**
  - 不负责失败后的重试策略（由恢复用例处理）。
  - 不直接处理人工协同。
  - 不覆盖插件内部执行逻辑。

# Solution Blueprint

## 体系分解

| 组件 | 责任 | 说明 |
|------|------|------|
| DAG Runtime | 解析 DAG、管理依赖与优先级 | 支持并行、串行、互斥节点及最大并发数。
| Sub-Agent Dispatcher | 子 Agent 任务分发、上下文注入 | 确保租户隔离、工具可用性校验、幂等任务领取。
| State Bus Streamer | 状态事件写入/订阅 | 发布 `agent.task.status.updated`，供调度与监控消费。
| Rebalance Manager | 动态调度、扩缩、副本协调 | 依据延迟、阻塞和失败率调整任务分配。
| Result Aggregator | 汇总部分结果、上下文与最终交付物 | 输出给主 Agent 及后续闭环用例。

## 流程与时序

1. **Step 1 – DAG 装载**：Orchestrator 读取任务 DAG，计算初始拓扑顺序与资源需求。
2. **Step 2 – 分发子任务**：Dispatcher 根据租户上下文与子 Agent 能力发放工作，注入 Trace/幂等键。
3. **Step 3 – 状态上报**：子 Agent 在关键节点将进度、耗时与部分结果写入状态总线。
4. **Step 4 – 调度调优**：Rebalance Manager 消费状态事件，检测阻塞/超时并执行重排或扩容。
5. **Step 5 – 结果汇总**：所有子任务完成后，由 Result Aggregator 合并数据并输出。

```mermaid
sequenceDiagram
  participant Orchestrator
  participant Dispatcher
  participant SubAgent
  participant StateBus
  participant Aggregator

  Orchestrator->>Dispatcher: 任务 DAG + 策略
  Dispatcher->>SubAgent: 分发子任务/上下文
  SubAgent->>StateBus: 状态事件/部分结果
  StateBus->>Orchestrator: 订阅进度
  Orchestrator->>Dispatcher: 调整调度策略
  SubAgent-->>Aggregator: 节点输出
  Aggregator->>Orchestrator: 汇总结果
```

# Contracts & Interfaces

- **Inbound**：
  - `POST /internal/agent/dag/{dag_id}/execute` — Planner 调用，传入 DAG、策略、上下文。
  - `EVENT agent.plan.created` — 触发自动装载。
- **Outbound**：
  - `EVENT agent.task.status.updated` — 状态事件，包含 `task_id`, `node_id`, `status`, `progress`, `latency`。
  - `POST /internal/plugins/{pluginId}/invoke` — 子 Agent 调用插件，附带租户与 Trace。
  - `EVENT agent.task.blocked` — 通知 Ops/调度层处理阻塞。
- **配置/脚本**：
  - `config/agent/subagents.yaml` — 子 Agent 能力注册表。
  - `config/agent/scheduler_policies.yaml` — 并行度、限流、超时策略。
  - `scripts/qa/dag-simulator.mjs` — DAG 执行模拟器。

# Implementation Checklist

| 项目 | 描述 | 状态 | Owner |
|------|------|------|-------|
| DAG 拓扑校验 | 构建循环检测、资源推导 | [ ] | Agent Platform Guild |
| 子 Agent 认证 | 保障任务领取强制租户校验 | [ ] | Ops Reliability Center |
| 状态总线 Schema | 统一 `agent.task.status.updated` payload | [ ] | Agent Platform Guild |
| 调度策略 | 根据延迟/失败率动态调优 | [ ] | Agent Platform Guild |
| 汇总日志 | 结果聚合与上下文快照入审计 | [ ] | Ops Reliability Center |

# Testing Strategy

- **单元**：DAG 拓扑排序、优先级决策、状态机转换、幂等领取。
- **集成**：Dispatcher + 真实子 Agent + 状态总线，验证并行执行、阻塞检测、重排逻辑。
- **端到端**：沙箱任务触发多插件并行，观察 Grafana 状态曲线与结果汇总。
- **压力/Chaos**：注入部分子 Agent 宕机、状态事件延迟 >3s，检查调度降级与限流策略。

# Observability & Ops

- **指标**：`agent.statebus.lag_ms`, `agent.task.parallelism`, `agent.task.blocked_total`, `agent.task.repeat_total`, `agent.result.generation_latency`。
- **日志**：分发决策、重排动作、子 Agent 分配历史、阻塞原因。
- **告警**：状态延迟 >1s、阻塞任务 >20、重复执行率 >0.5%；通知 Ops 值班。
- **Dashboard**：Grafana「Agent Execution」、Ops 控制台任务看板、Datadog `agent.statebus.*`。

# Rollback & Failure Handling

- DAG Runtime 升级可通过蓝绿切换，异常时退回旧版本并暂停新任务。
- 状态总线不可用时降级为数据库轮询并限制最大并发。
- 子 Agent 批量失败时触发 `agent-exec-pause` Flag，阻止新任务进入。

# Follow-ups & Risks

| 风险 | 影响 | 缓解 | ETA |
|------|------|------|-----|
| 子 Agent 注册表未与插件发布联动 | 任务领取失败 | 接入插件健康信号与版本事件 | 2025-03-08 |
| 状态事件 schema 变更未通知下游 | 指标面板受影响 | 发布 schema 版本与兼容层 | 2025-03-01 |

# References & Links

- 场景文档：`docs/scenarios/agent-orchestration/SCN-AGENT-TASK-EXEC-001.md`
- 插件健康度标准：`docs/standards/powerx/backend/integration/09_agent/Agent_Metrics_and_Observability.md`
- 调度 Runbook：`scripts/qa/workflow-metrics.mjs`

---
scn_id: "SCN-AGENT-TASK-EXEC-001"
scenario_name: "智能体任务执行"
slug: "powerx-agent-task-execution"
primary_scope: "powerx"
primary_layer: "integration"
primary_domain: "agent-orchestration"
primary_repo: "powerx"
doc_owner: "Agent Platform Guild（Scenario Steward / agent-platform@artisan-cloud.com） & Ops Reliability Center（Automation Co-owner / ops-center@artisan-cloud.com）"
last_generated_at: "2025-11-09"
---

# 智能体任务执行 Usecase Seed 生成指南（Stage 2）

> 场景摘要：主 Agent 将 Planner 生成的任务 DAG 映射到多子 Agent 并行执行，依赖状态总线与调度策略在 <1 秒延迟内感知状态、在 5 分钟内完成阻塞处理，并维持 95%+ 的任务成功率。

本文档面向场景负责人与仓库 Stewards，说明如何把 `UC-AGENT-EXEC-COORD-001` 等 Stage 2 子用例拆解成可交付 Seed，为分发脚本与研发落地提供基础数据。请根据实际情况补充或修订所有 `{{PLACEHOLDER}}` 字段。

## Seed 的定位

- 代表 PowerX 核心仓在 `智能体任务执行` 场景的并行执行/状态协调职责。
- 与 `docs/_data/docmap.yaml` 中 `scn_id: SCN-AGENT-TASK-EXEC-001` 的 `children` 节点一一对应，字段必须保持一致。
- 是 `npm run publish:usecases`、`npm run publish:collected` 读取 Stage 2 配置的唯一来源，驱动多仓对齐调度策略、指标与 Runbook。

## 前提条件

- 场景文档 `docs/scenarios/agent-orchestration/SCN-AGENT-TASK-EXEC-001.md` 已定义 Stage 2 流程、参与者与指标。
- `docs/_data/docmap.yaml` 已登记 `UC-AGENT-EXEC-COORD-001` 的 `scope: powerx`、`layer: integration`、`domain: agent-orchestration`、`repo: powerx` 等字段。
- `docs/_data/repos.yaml` 中 `powerx` 仓库配置了 `usecase_seed_root: docs/use_cases/_from_hub` 与默认分支 `dev/docs`，供脚本投递。
- Feature Flag `agent-orchestrator-v2`、`statebus-stream`、`scheduler-autoscale`、`workflow-trigger-kit`、`copilot-handoff` 已在目标环境启用；Kafka/SQS 状态总线、子 Agent 注册服务、插件健康信号源、Copilot 工单桥均可用。
- 观测/验证工具（Grafana「Agent Execution」、`scripts/qa/workflow-metrics.mjs`、`scripts/qa/dag-simulator.mjs`）为最新版本，便于验证 Seed 描述的指标与流程。

> **TODO**：若需要额外依赖（例如新的租户配额服务、异常检测模型或临时凭据），请在此列出以便审计。

## 生成流程

1. **登记/更新 docmap 子节点**

   ```yaml
   # docs/_data/docmap.yaml
   - scn_id: SCN-AGENT-TASK-EXEC-001
     title: 智能体任务执行
     children:
       - doc_id: UC-AGENT-EXEC-PLAN-001
         scope: powerx
         layer: service
         domain: agent-orchestration
         optional: false
         repo: powerx
         path: docs/use_cases/_from_hub/SCN-AGENT-TASK-EXEC-001/UC-AGENT-EXEC-PLAN-001.md
       - doc_id: UC-AGENT-EXEC-COORD-001
         scope: powerx
         layer: integration
         domain: agent-orchestration
         optional: false
         repo: powerx
         path: docs/use_cases/_from_hub/SCN-AGENT-TASK-EXEC-001/UC-AGENT-EXEC-COORD-001.md
       - doc_id: UC-AGENT-EXEC-RECOVERY-001
         scope: powerx
         layer: ops
         domain: agent-orchestration
         optional: false
         repo: powerx
         path: docs/use_cases/_from_hub/SCN-AGENT-TASK-EXEC-001/UC-AGENT-EXEC-RECOVERY-001.md
       - doc_id: UC-AGENT-EXEC-CLOSURE-001
         scope: powerx
         layer: ops
         domain: agent-orchestration
         optional: false
         repo: powerx
         path: docs/use_cases/_from_hub/SCN-AGENT-TASK-EXEC-001/UC-AGENT-EXEC-CLOSURE-001.md
   ```

   - `doc_id` 与 Seed 文件名、下游仓库 `path` 必须一致，避免发布脚本遗漏。
   - 如需给 `powerx-plugin` 等其它仓库扩展并行执行能力，请新增子节点并保持字段同步。

2. **复制模板并放置到对应目录**

   ```bash
   mkdir -p docs/usecases-seeds/SCN-AGENT-TASK-EXEC-001
   cp docs/usecases-seeds/_template.md \
     docs/usecases-seeds/SCN-AGENT-TASK-EXEC-001/UC-AGENT-EXEC-COORD-001.md
   ```

   - `scope/layer/domain` 目录层级需与 docmap/Frontmatter 一致。
   - 若从老路径迁移（`docs/usecases-seeds` → `docs/use_cases/_from_hub`），请同步更新 docmap `path`。

3. **填充 Frontmatter 区域**

   - `repo_key` 使用 `repos.yaml` 的 `powerx`，`scenario_title` 统一为「智能体任务执行」。
   - `owners` 默认沿用 Agent Platform Guild + Ops Reliability Center；如引入子 Agent 平台或插件团队，请在 `contributors` 中列明。
   - `feature_flags` 至少列出 `agent-orchestrator-v2`、`statebus-stream`、`scheduler-autoscale`，并补充 `workflow-trigger-kit`、`copilot-handoff` 等 Stage 2 依赖。
   - `code_refs` 应覆盖 `services/agent/orchestrator/dag_runtime.ts`、`services/agent/subagents/dispatcher.ts`、`services/statebus/event_streamer.ts`、`services/agent/controller/rebalance_manager.ts` 等关键模块。

4. **完善正文章节**

   - `Usecase Overview`：突出并行成功率、状态延迟、阻塞检测的 SLA，与 Stage 2 角色（主 Agent、子 Agent、状态总线）的关系。
   - `Context & Assumptions`：写明 DAG 输入、子 Agent 注册表、状态事件 schema、非目标（人工协同、失败恢复）。
   - `Solution Blueprint`：描述 DAG Runtime、Dispatcher、State Bus、Rebalance、Result Aggregator 等组件及 Mermaid 流程。
   - `Contracts & Interfaces`：列出 `POST /internal/agent/dag/{dag_id}/execute`、`EVENT agent.task.status.updated`、状态事件字段、配置文件与模拟脚本。
   - `Testing / Observability / Rollback`：纳入 DAG 模拟、状态延迟监控、`agent-exec-pause` Flag、Copilot 工单触发条件等。

5. **与场景文档互相链接**

   - 在 `docs/scenarios/agent-orchestration/SCN-AGENT-TASK-EXEC-001.md` 的 Stage 2 章节或交付矩阵中补充 Seed 链接。
   - 若引用新的指标或标准，请同步更新 `docs/standards/powerx/backend/integration/09_agent/Agent_Metrics_and_Observability.md` 或新增标准文件。

## 自检清单

- 已确认 `docmap.yaml`、Seed Frontmatter、下游仓库路径完全一致。
- Seed 正文覆盖调度策略、状态事件、阻塞检测、结果汇总、观测指标与降级流程。
- `scripts/qa/dag-simulator.mjs`、`scripts/qa/workflow-metrics.mjs` 在最新实现下可运行，并与 Seed 中的指标/流程一致。
- Feature Flag 与外部依赖（状态总线、子 Agent 注册、Copilot 桥）全部列在前置条件/上下文中，并提供可观测项。
- 执行 `npm run lint` 与 `npm run docs:build` 验证 Markdown/站点构建无误。
- 使用 `npm run publish:scenarios -- --scn-id SCN-AGENT-TASK-EXEC-001 --validate-only` 检查场景配置后，再向下游仓库发布。

## 常见问题

| 问题 | 处理方式 |
|------|----------|
| 状态总线延迟 >1 秒导致调度失效？ | 在 Seed 中补充 `statebus-lag` 告警、隔离队列/优先级策略，并提供临时降级（数据库轮询、降低并行度）的运行手册。 |
| 子 Agent 注册表未及时同步插件版本？ | 描述与插件健康信号/发布事件的联动流程，要求 Seed 引用自动化脚本或 Runbook，并在 Follow-ups 中记下治理计划。 |

完成上述步骤后，即可继续执行场景分发及领导力视图生成，参考《发布 Usecase Seeds 指南》获取后续操作。
