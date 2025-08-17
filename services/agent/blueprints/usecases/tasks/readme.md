我们先把“大前提”对齐，然后给一套**可落地的最小架构与数据结构**，把“多意图→多目标→任务(蓝图)→多轮 ReAct 执行+人机协同(HITL)”串起来。最后给一个**端到端小例子**与**最小实现清单**。

# 0. 共识（回答你的两个判断）

**✅ 共识成立**

* **一次用户请求**可能包含**多个意图**（Intents）。
* 每个意图对应**一个或多个目标**（Goals）。
* 每个目标由\*\*≥1 个任务（Task/Flow=蓝图）**达成；任务内部由**节点(步骤)**组成，允许**子任务/子流\*\*。
* **ReAct** 是运行时范式：**思考(Thought)** → **行动(Action/调用 handler/工具)** → **观察(Observation)** →（必要时）**追问用户/审批(HITL)** → **继续/修正计划**。
* **多轮**是常态：中途可能需要用户提供信息或做决策。

> 换言之：**“多意图→多目标→任务图(由若干 Flow 组成)→ReAct 多轮执行”** 是主线。

---

# 1. 概念分层（最小但足够）

* **Intent（意图）**：对用户话语的功能性归类，可**多选**。
* **Goal（目标）**：可验收的结果定义（含判定条件与关键槽位/约束）。
* **Task/Flow（任务/蓝图）**：实现某类目标的**可执行图**（节点+边，支持并行/条件/汇合/Map-Reduce/Saga/HITL）。
* **Plan（执行计划，运行时结构）**：把“本轮识别到的多个 Flow”拼成**任务间的 DAG**（可以不落盘，但运行时一定会构造这张图）。

> 你可以继续按“**用例=文件夹**”管理蓝图；**Plan 只是运行时拼装出来的那张任务间图**（编排态），不强制单独存文件。

---

# 2. 运行时生命周期（ReAct + 多轮）

1. **识别阶段**

* NLU → **多意图打分** → 选择 Top-K（带阈值/相似度去重）。
* **意图→目标**：为每个意图产出 1..m 个目标（补齐缺失槽位）。

2. **计划阶段**

* 目标映射到**Flow 模板**（蓝图库），实例化成 **task\_i**；
* 结合依赖与共享数据，**拼装任务间 DAG（Plan）**；
* 如槽位缺失/高风险动作 → 立即进入**HITL/澄清问题**（多轮）。

3. **执行阶段（ReAct 循环）**

* Planner 选定下一个可运行节点/任务 → 执行 handler（Action）→ 产出 Observation；
* 根据 Observation 更新上下文/记忆/判定 → 继续/改线/回退/重试；
* 碰到 **HITL** 或需要补料 → **暂停等待用户**；收到回复后恢复。

4. **收尾阶段**

* 验收每个目标的“完成条件”（Goal 的 OK 判定）；
* 出具结果卡/留痕/可回放轨迹。

---

# 3. 关键数据结构（建议的最小 Schema）

## 3.1 Intent（多意图输出）

```json
{
  "intents": [
    { "id": "lead_create", "score": 0.92, "entities": {"company":"ACME"} },
    { "id": "publish_posts", "score": 0.81, "entities": {"platforms":["xhs","douyin"]} }
  ],
  "missing_slots": [
    { "goal":"publish_posts", "slot":"assets", "ask":"请上传2张图或给我素材链接？" }
  ]
}
```

## 3.2 Goal（从意图规范化）

```json
{
  "goals": [
    { "goal_id":"G1", "intent_id":"lead_create",
      "acceptance": {"created": true, "lead_id": "string"} },
    { "goal_id":"G2", "intent_id":"publish_posts",
      "acceptance": {"posts_published": 2} }
  ]
}
```

## 3.3 Flow（任务/蓝图，任务内是**图**）

> 你已有的 `nodes`/`use`/`params` 很好；**补上 `edges`** 支持并行/条件/汇合即可。

```yaml
flow_id: task_2
metadata:
  io:
    inputs: [{ name: upstream_id, type: string, required: true }]
    outputs: [{ name: id, type: string }]

nodes:
  - id: n1
    use: core.debug.forward
    kind: biz_logic
    io:
      in:  { upstream_id: "{{ input.upstream_id }}" }
      out_map: { id: id }
  - id: n2
    use: core.response.format
    kind: text_proc
    params: { template: '{ "id":"{{ node.n1.output.id }}","from":"t2"}' }
    io:
      out_map: { id: id }

edges:
  - { from: n1, to: n2, type: SEQ }   # 线性；需要并行/条件/汇合时写 JOIN/COND/...
```

## 3.4 Plan（运行时“任务间 DAG”，可不落地文件）

```json
{
  "plan_id": "sess_123_plan",
  "tasks": [
    { "tid":"t1", "flow_id":"task_1" },
    { "tid":"t2", "flow_id":"task_2", "binds":{"input.upstream_id":"{{ task.t1.output.id }}"} },
    { "tid":"t3", "flow_id":"task_3" }
  ],
  "edges": [
    { "from":"t1", "to":"t2", "type":"SEQ" },
    { "from":"t2", "to":"t3", "type":"SEQ" }
  ],
  "policy": { "concurrency": 4, "trace": {"replayable": true} }
}
```

> **要点**：你继续把蓝图按“用例=文件夹”维护完全没问题；**Plan 是把这些蓝图在运行时按本轮意图/目标组合起来的结构**。它存在于会话态（内存/数据库），不与“用例目录”冲突。

---

# 4. 多轮与 HITL（如何“问”与“等”）

在 ReAct 里，**问用户**也是一种 Action。给两个标准节点：

* `use: human.ask`（**澄清节点**）：输出 `question`，流转到 `PAUSED`；收到用户答复事件 `user_reply` → 写回 `vars.xxx` → 继续。
* `use: human.approval`（**审批节点**）：输出 `approved: true|false`；支持 `ttl`、超时策略（自动拒绝/走降级）。

**状态机（节点/任务）**
`READY → RUNNING → (PAUSED[HITL] | SUCCEEDED | FAILED) → (RESUMED | COMPENSATING | CANCELED)`。

---

# 5. 从“多意图一句话”到“执行完成”的小例子

**用户**：

> “给 ACME 创建一个新线索并在明天 10:00 安排电话；同时把今天写好的两条小红书草稿发出去。”

**(1) 多意图识别**

* `lead_create`（score 0.92, company=ACME, time\_slot=明天10点）
* `publish_posts`（score 0.84, platforms=\[xhs], slots 缺 `assets` → 需要追问）

**(2) 生成目标**

* G1: 线索已创建 & 任务已排程
* G2: 小红书已发布两条

**(3) 拼装 Plan（运行时）**

* 选择 Flow：`task_lead_create` → `task_schedule_call`；`task_publish_post`（×2，Map 扩展）
* 任务间边：`t_lead → t_schedule`（SEQ）；`t_publish[*]` 与其它任务**并行**
* 因缺 `assets`：在 `t_publish` 前插入 `human.ask` → `PAUSED`

**(4) ReAct 循环（节选）**

* Thought: 缺素材 → Action: `human.ask` → Observation: 等待
* 用户上传两张图 → Observation: `assets=…` → 继续
* 并行执行：创建线索/排会 + 生成图文/发布 → 汇总 → 输出完成卡片

**(5) 完成判定**

* `lead_id` 存在 & 日程条目存在；`posts_published=2` → 成功

---

# 6. 你的蓝图如何对齐（最小改造指南）

* **保留你现有的写法**：`nodes(use/kind/params/io)` 不动。
* **新增** `edges` 到 **Flow 内部**，以支持并行/条件/JOIN。
* **Plan 只作为运行时结构**（可存在内存/DB），把“**本次会话识别到的多个 Flow**”按需拼接。
* 继续使用你喜欢的“**用例=文件夹**”目录来存放蓝图；需要对比不同关系链时，每个用例文件夹附带一份 case 级的 `edges`（可选）用于**测试**。生产运行时由引擎**根据识别结果动态生成 Plan**，不强制读取 case 文件。

---

# 7. 最小实现清单（接口/模块）

* `DetectIntents(input) -> intents[]`（规则+LLM 双路，支持多意图）
* `IntentsToGoals(intents[]) -> goals[]`（补齐槽位，产出缺失项）
* `SelectFlows(goals[]) -> task_instances[]`
* `AssemblePlan(tasks[]) -> plan`（推断任务间的 Edges & Binds）
* `ExecutePlan(plan, ctx)`（ReAct 循环：调度节点、产 Observations、命中 HITL 时暂停；收到 `user_reply/approval` 事件后 `Resume`）
* `Persist(state)`（会话态/轨迹/回放）
* `HITL.Webhook/Inbox`（收集人机交互事件）

---

# 8. 你可能关心的两点

* **“Plan vs 用例目录”**：Plan 是**运行时拼图**，不是强制的文件层；你的“用例目录”更像**脱机测试集**。两者可并存、互不替代。
* **“只用 required 不引入 edges 行不行”**：只够线性/顺序；一旦要并发/竞争/条件/Map/Saga/HITL，就**必须**在 Flow 内部引入 `edges`（或等价表达）。

---