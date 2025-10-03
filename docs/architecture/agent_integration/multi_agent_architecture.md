# PowerX 多 Agent 技术架构方案（A2A / Agent-to-Agent）

本方案基于 `internal/server/agent` 现有实现，阐述如何在当前引擎之上扩展出「单任务多 Agent 协同」能力，覆盖架构目标、核心组件、执行流程、数据流、协议扩展与迭代路径。

配套的交互体验请参见《[PowerX 多 Agent 产品体验方案](./multi_agent_product_experience.md)》，两份文档可结合使用以指导端到端落地。

## 1. 架构目标与边界

1. **单任务多参与者**：同一 `session_id` 可以在一次 run 中调度多个 Agent（Planner、Executor、Reviewer 等），形成自治协作网络。
2. **可控执行图**：沿用 `schemas.ExecutionPlan`，在 Plan 中允许多个 Agent 节点，支持串行、并行与依赖自动补齐。
3. **全链路可观测**：每个 Agent 的开始、增量、完成事件在 SSE/WS 中清晰可见，可用于 run 回放。
4. **渐进演进**：最大化复用现有组件，优先在 Engine 层补足协同逻辑，后续再抽象独立 orchestrator。

## 2. 现有能力回顾

- **Manager**（`manager.go` 等）：负责 Agent 注册、Flow 路由、运行时状态维护，已具备 `DetectTasks` 与 `BuildPlan`。
- **Intent & Blueprint**：`blueprints/`、`intent/`、`schemas/` 描述 Flow、步骤、参数、依赖，通过 `RegisterFlowRoute` 绑定 Agent。
- **Runtime.Engine**（`runtime/engine.go`）：串起意图识别 → 计划生成 → Flow 选择 → `AgentClient.Stream`，并通过 `EventSink` 输出 SSE/WS。
- **驱动与适配器**：`drivers/`、`adapters/`、`contract/` 封装模型、工具、消息格式。
- **运行日志与回放**：`history_view.go`、`runtime/sink_*` 负责 run 级别的事件回放。

> 现状结论：核心的「多意图识别 + Plan 构造 + Agent 路由」已具备；尚需构建**会话级协作上下文与生命周期管理**以支撑多 Agent 调度。

## 3. 协作关系模型

为了回答「多 Agent 之间的组织关系是什么」这一核心问题，本方案将会话拆解为**编排器（Orchestrator）+ 参与者（Agent）**两层：

1. **SessionArena = 编排器**：负责会话生命周期、Plan 调度与共享上下文。Arena 不直接产出业务结果，而是调度 Agent 完成。Arena 的实现细节见第 4 章。
2. **Coordinator Agent（可选角色）**：当业务希望保留“主 Agent”体验时，可在 Plan 的首个 Stage 放置 Coordinator。它负责拆解用户需求、再派发给其他 Agent。若无需 Coordinator，可直接由 Arena 按 Plan 调度 Worker Agent。
3. **Worker Agent**：执行具体任务的参与者，对应既有注册的 Flow / Handler / MCP 插件等。
4. **Observer / Reviewer Agent**：对其他 Agent 的输出做校验、评审或补充，可异步监听 `agent.delta` 并在 Plan 中设置依赖。

> 关系总结：Arena 扮演“裁判台”，负责调度；Coordinator（如有）扮演“主 Agent”进行任务拆解；其余 Agent 以平级身份协作，可通过 Plan 定义串行/并行依赖，不再强制单主从模型。

## 4. 核心新增组件

| 对象 | 建议路径 | 主要职责 |
| --- | --- | --- |
| `SessionArena` | `internal/server/agent/arena/session.go` | 管理单个会话的 Agent 参与者、Plan 执行状态与上下文共享。|
| `Participant` | `arena/participant.go` | 封装 `AgentClient`、Flow 元信息与角色标签。|
| `Turn` | `arena/turn.go` | 表示一次 Agent 执行记录，包括输入、输出与状态。|
| `PlanRunner` | `arena/runner.go` | 执行 `ExecutionPlan`，调度多 Agent 并发/串行执行，聚合结果。|
| `MultiSink` | `runtime/sink_multi.go` | 在现有 SSE/WS sink 之上增加参与者标签与阶段事件。

所有新增对象统一置于 `internal/server/agent/arena/` 下，命名风格与现有目录保持一致，便于后续模块化。

### 4.1 与现有单 Agent 执行的衔接

- 现阶段 Engine 基于「多任务识别 → 单 Agent 执行」实现，`ExecutionPlan.Tasks` 的每个节点可能是 Tool、Handle 或注册的 MCP。
- 多 Agent 扩展后，**Plan 节点类型新增 `AgentNode`**：当节点声明 `type = agent` 时，由 Arena 通过 `Participant` 调用该 Agent；其余类型保持兼容。
- 若业务沿用旧流程，只要 Plan 中没有 `AgentNode`，Arena 会退化为单 Agent 模式，保持向后兼容。

### 4.2 Agent 作为 Workflow 节点

Plan 构造阶段可将 Agent 视为 Workflow 节点，与 Tool/Handle 统一建模：

```text
ExecutionPlan
  ├─ Task(type=agent, agent_id="coordinator", stage=0)
  ├─ Task(type=agent, agent_id="code_writer", stage=1)
  ├─ Task(type=tool,  tool_id="github.create_pr", depends_on=[code_writer])
  └─ Task(type=agent, agent_id="reviewer", stage=2, depends_on=[code_writer])
```

`Manager.BuildPlan` 只需在识别到复杂任务时插入以上节点，即可完成“Agent 即节点”的建模，Arena/PlanRunner 会据此自动调度。

## 5. 多 Agent 会话执行流程

1. **接入**：客户端沿用 `docs/agent/communicate.md` 协议，通过 WS 发起 `run.start`，引擎创建 `SessionArena` 并打开 SSE。
2. **意图识别**：Arena 调用 `Manager.DetectTasks` 获得多个 `DetectedTask`，`Task.AgentID` 指向候选 Agent。
3. **计划生成**：Arena 调用 `Manager.BuildPlan`，得到包含多节点 Flow 的 `ExecutionPlan`。
4. **参与者映射**：依据 Plan 节点中的 `AgentID` 取出 `AgentClient`，包装为 `Participant`；缺省回落到默认 Agent。
5. **运行调度**：
   - `PlanRunner` 遍历 `ExecutionPlan.Tasks`，按 `Stage` 控制串行/并行。
   - 调用 `Participant.Stream` 获取增量事件，由 `MultiSink.Emit` 输出 `agent.started/delta/completed`。
   - 节点完成后将结构化结果写入 `SessionArena.ContextStore` 供后续节点消费。
6. **聚合与终止**：全部节点完成后，Arena 汇总最终回复（可由最后节点或独立 Reviewer 产出），发出 `run.completed`。
7. **记忆与回放**：Arena 将 Plan、Turn 与输出写入现有 `history_view` 或新表用于回放与分析。

## 6. 会话数据结构

```text
SessionArena
  ├─ ContextStore: map[string]any        // 共享上下文（用户输入、Agent 结果）
  ├─ Participants: map[string]Participant// Agent 实例，key = AgentID 或 FlowID
  ├─ Turns: []Turn                       // 执行历史
  ├─ Plan: schemas.ExecutionPlan         // 当前执行图
  └─ Sink: runtime.EventSink             // SSE/WS 输出
```

- **输入上下文**：首个节点使用用户消息，后续节点从 `ContextStore` 读取上游产出（沿用 Flow 参数连线规则）。
- **输出合并**：每个 `Turn` 记录 `raw_chunk`、`final_result`、`observations`，支持溯源与学习。
- **跨 Turn 数据**：以 `flow_id` 或 `task_id` 为 key 写入 `ContextStore`，并同步到 `Turn.Output` 便于回放。

## 7. 调度策略（同步 + 异步结合）

1. **串行执行（同步）**：默认按 `Stage` 升序串行，适用于 Coordinator → Worker → Reviewer 的主干链路。
2. **并行执行（异步）**：无依赖节点由 goroutine 并行执行。`PlanRunner` 通过 `errgroup` 等机制等待并收敛结果，实现“既有异步又有同步”。
3. **事件流异步广播**：即便串行执行，`Participant.Stream` 产生的 `agent.delta` 会实时推送给前端，其他 Agent 也可选择监听（如 Reviewer 根据增量做实时反馈）。
4. **动态插入节点**：利用现有 `auto_prereq` 逻辑执行前扩展 Plan（如 Reviewer 需要摘要），Arena 支持在运行中追加 `AgentNode`，并通过 `run.inject_agent` 控制面让人工干预。
5. **超时与取消**：Arena 统一持有 `context.Context`；WS 收到 `run.cancel` 时取消整个 Plan，节点执行传入派生 context。并发节点中任一失败可按策略（全部取消或继续）配置。

## 8. 插件化 Agent 的接入

- 插件安装时通过 `Manager.Register` 注册为 Agent，具备 `FlowID` 与 `IntentSpec`。
- 多 Agent 模式下，Plan 节点包含该 Flow/Agent 即可调度插件。
- 若需租户隔离，在 `Participant` 中附带 `tenant_id` 与插件授权（复用 `contract.Capability` 验证）。
- `Capability Registry` 可新增 `multi_agent_role` 字段，用于自动分配角色与优先级。

## 9. SSE/WS 协议扩展

- **新增事件**：
  - `agent.started`：`{agent_id, task_id, stage, plan_id}`
  - `agent.delta`：`{agent_id, task_id, delta, step_id, metadata}`
  - `agent.completed`：`{agent_id, task_id, output, observations}`
  - `agent.failed`：`{agent_id, task_id, error}`
- **关联字段**：所有事件携带 `run_id`, `session_id`, `plan_id`, `turn_seq`，保证回放。
- **控制面**：支持 `run.pause/resume`、`run.inject_agent`（动态加入 Agent）、`run.override_plan`（人工编辑 Plan）。

## 10. 迭代计划

**MVP（兼容现有 Engine）**

1. 在 `runtime/` 新增 `sink_multi.go`，封装带 Agent 元信息的事件输出。
2. 在 `arena/` 包实现 `SessionArena`、`PlanRunner`，重构 `Engine.Run`：创建 Arena → 生成 Plan → `PlanRunner.Execute` → 聚合结果。
3. SSE/WS 兼容新增事件，未开启多 Agent 时不发送。

**M2：插件与角色管理**

1. `capability` 模块新增多 Agent 元数据（角色、成本、优先级）。
2. Intent 识别引入角色倾向（如“需要代码审查”→优先选择 Reviewer）。
3. `run.start` 支持显式指定参与 Agent，Arena 校验并加载。

**M3：自适应编排与记忆**

1. 根据历史 `Turn` 结果调整后续 Plan（简易自学习）。
2. `history_view` 增强多 Agent 时间线回放。
3. Reviewer Agent 提供反馈写入知识库。

## 11. 与 Google A2A 的对齐

- **统一协议**：复用 `contract.AgentClient.Stream`，与 Google A2A 的统一契约一致。
- **协作自治**：各 Agent 通过 Plan 独立执行，同时共享 Arena 上下文协同。
- **可观测链路**：多 Agent 事件进入统一 SSE/WS 通道，支撑 DAG/泳道可视化与调试。
- **渐进式演进**：先落地多 Agent 执行，再迭代策略与学习层。

## 12. 落地建议

1. **代码改动顺序**：
   - `internal/server/agent/runtime`：拆分 Engine，增加多 Agent Sink。
   - `internal/server/agent/arena`：新建包实现会话与 Plan 执行。
   - `internal/server/agent/manager_execute.go`：按需新增多 Agent 路由 API（如按角色查询 Agent）。
2. **测试策略**：
   - 单元测试覆盖 `PlanRunner` 的串行/并行/取消流程。
   - 集成测试在 `internal/server/agent/test` 构造多 Agent 场景（两个虚拟 Agent 输出合并）。
   - 回归单 Agent 流程确保兼容。
3. **上线控制**：提供 Feature Flag（如 `multi_agent.enabled`）分阶段灰度。

---

通过以上技术架构扩展，可在现有 Engine 内快速实现 PowerX 的多 Agent 协作（A2A）能力，并为后续策略、学习、监控等能力打下基础。
