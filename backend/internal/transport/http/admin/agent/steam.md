
---

# 我的理解（落地版）

## 目标

* **唯一对外聊天入口**：`GET /agents/stream/flow`（SSE）
* **前端只传自然语言 `q`**（可选 `session_id`），**不传 flow\_id**
* 后端在一次请求里完成：

    1. **DetectTasks**（多意图识别）
    2. **BuildPlan**（把 task 组装成“可执行 flow 列表”）
    3. **按顺序执行每个 flow**（每个 flow 内部是多个 step/node）
    4. **全程以 SSE 事件推送**：intent → plan → 每个 flow 的 start/progress/end → token/data → final → end
    5. **与 session 关联并写历史**（user + 合并的 assistant 输出；按会话做摘要/裁剪）

## SSE 事件协议（建议，兼容你现有 dto）

* `start`：回 `session_id`、`request_id`、`ts`
* `intent`：返回任务识别结果（可能是 `matched=false`）
* `plan`：返回计划（flow 队列），例如 `[{flow_id, name, args}]`
* `flow_start`：每个 flow 开始（包含 `flow_id/index/total`）
* `token` / `data`：由当前 flow 的执行流透传（沿用你现在的 `WriteToSSE`）
* `flow_end`：每个 flow 结束（success/fail、耗时、产物摘要）
* `final`：整个计划完成后的汇总回答（用于渲染最终 assistant 气泡）
* `end`：整个请求结束（ok=true/false）
* `error`：错误帧（不中断/中断都要发）

> 注：SSE 不适合“外部取消”的控制，所以**取消/心跳**走你已经保留的 `WS` 通道（`/agents/stream/ws`）。SSE 这边只消费“执行结果”。

## 决策与兜底

* **只要 `flow_id` 未指定** → 始终走 **DetectTasks → BuildPlan → 执行计划**。
* **若 DetectTasks 结果为空**：

    * 尝试“**fallback flow**”（比如 `chat` 模板 flow）。
    * 如果你的系统里确实**没有注册** `chat`，则发：

        * `intent`（matched\:false, reason）
        * `error`（`flow_not_found`，并可附上 `available_flows` 列表）
        * `end`（ok\:false）
* **有 `session_id`** → 附加在执行 meta/context + 结束时写库；
  **没有 `session_id`** → 根据 agent 策略（是否 singleton）**创建一个会话**，在 `start` 帧里回传 `session_id`，后续前端沿用。

## 执行模型（“队列”的具体含义）

* 你说的“队列”更像**本次请求内部的顺序调度**（不是跨请求的 broker）。落地上：

    * 就是在 `streamCore` 里对 `plan.flows` **for 循环**串行执行；
    * 每轮调用 `ag.Stream(ctx, flowID, params, meta)`，在外面包一层：先 `flow_start`，再把 reader 交给 `WriteToSSE` 透传 token/data，最后 `flow_end`；
    * 任一 flow 报错 → 发 `flow_end{success: "false}`，视策略决定是否“短路退出”或“跳过继续下一个 flow”（建议默认短路）。"

## Session 写库

* 本轮对话结束：

    * `AppendPair(sessionID, agentID, userText, assistantTextAll)`（assistant 内容是把各 flow 的“可渲染输出”拼成一段，或取最后一个 flow 的产物）
    * `SummarizeIfNeeded` 超限即做滚动摘要
* `session_id` 的产生/传递逻辑：

    * 如果 query 里没有：`GetOrCreateSession(env, tenant, agentID, userID, singleton, defaults)` → `start` 帧回传；
    * 如果有：验证作用域后直接使用。

---

# 需要改动的点（针对你贴的代码）

1. **`StreamFlow` → 只接 `q`（和可选 `session_id` / `preview`）**

    * 去掉 `flow_id` 参与；只保留 `preview=1` 时“识别但不执行”（用于调试/演示）。
2. **`streamCore` → 内部改成**：

    * 打开 SSE 头 → `start`（含 `session_id`、`request_id`）
    * `DetectTasks`（多意图）→ `intent`
    * `BuildPlan` → `plan`
    * `for plan.flows`：`flow_start` → `WriteToSSE(...)` → `flow_end`
    * 汇总回答 → `final` → 写库（AppendPair + SummarizeIfNeeded）→ `end`
3. **取消 `flowID` 的兜底“直接覆盖”逻辑**

    * 现在的 `flowID := req.FlowID` / `intent.FlowID` / `if empty -> chat` 这块要替换为“**plan 驱动**”；只有当 `DetectTasks` 为空时，才 fallback 到 `chat`（且要检查是否注册，否则发错误事件）。
4. **加 `session` 处理**

    * 取 `session_id`（query/body），没有则 `GetOrCreateSession`；
    * `start` 帧把 `session_id` 返回；
    * 结束时写库。
5. **WS 取消**

    * 现在先不改 SSE；保留 `WS` 用于 cancel（后续在 `AgentWSHandler` 把 cancel 信号与执行协程的 context 绑定即可）。

---

# SSE 事件样例（一次完整执行）

```
event: start
data: {"session_id":123,"request_id":"req_xxx","ts":...}

event: intent
data: {"matched":true,"tasks":[{"name":"search","args":...},{"name":"summarize"}]}

event: plan
data: {"flows":[{"flow_id":"search_flow","args":...},{"flow_id":"summarize_flow"}]}

event: flow_start
data: {"index":1,"total":2,"flow_id":"search_flow"}

event: token
data: {"delta":"..."}

event: flow_end
data: {"index":1,"flow_id":"search_flow","success":true,"elapsed_ms":382}

event: flow_start
data: {"index":2,"total":2,"flow_id":"summarize_flow"}

event: token
data: {"delta":"..."}

event: final
data: {"content":"最终合成回答..."}

event: end
data: {"ok":true}
```

---

# Fallback 行为（没识别到意图）

* `intent`：`matched:false, reason:"no task detected"`
* 如果系统注册了“`chat`”或“`default_dialog`”之类的兜底 flow：

    * `plan`：`flows:[{"flow_id":"chat"}]` → 正常执行
* 否则：

    * `error`：`{"code":"flow_not_found","message":"fallback flow not registered","candidates":[...]}`
    * `end`：`ok:false`

> 这里我建议：要么**注册一个最小的 `chat` flow**（纯 LLM 对话），要么把 fallback flow id 做成**可配置**（缺省值在 `Agent` 级 Setting）。

---

# 小结

* **StreamFlow** 成为“识别 → 计划 → 队列执行 → SSE 推流”的唯一入口；
* **不再从前端传 flow\_id**；
* **会话 session** 在这里创建/复用并写历史；
* **WS** 专职控制（取消/心跳），SSE 专职内容。

“plan 驱动 + session 写库”的版本，并给出最小可跑的代码块（沿用你已有的 `ChatHistoryService` 与 `WriteToSSE`），

这样你可以马上在 Insomnia 用 `GET /agents/stream/flow?q=...` 验证从 `intent→plan→flow_start→token...→final→end` 的完整链路。
