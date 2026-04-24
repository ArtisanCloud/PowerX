# L9 - 多任务协作验收用例（页面可见 + 后端可查）

## 文档目的

这份文档只回答一件事：如何证明“TL + 子智能体”协作链路真正跑通。  
不是团队配置教程。

## 当前状态（2026-04-23）

最小场景已经可以测试。  
建议优先走页面验收，不要求手工拼 `team_id=1` 这类 URL。

统一入口（按优先级）：

1. 团队任务页：`/agent/team-tasks`（页面内团队选择器选团队）
2. 会话消息卡片：助手消息中的“执行过程”区块
3. Skills 页面 A2A 审计抽屉：`/settings/ai/skills` → `按 Team 查看 A2A 审计`
4. 审计接口（补充核验）：`GET /admin/skills/traces`

---

## 与当前实现对齐的边界

1. TL 唯一承担 `planner`。
2. 子智能体角色只允许 `retriever/executor/reviewer`。
3. 子智能体必须满足：同租户、`active`、非系统、非内置。
4. 子成员优先级不再手工配置，统一默认值。
5. 页面“执行过程”当前可直接看到：
`Intent：候选 N 个`、`Plan：节点 M 个`、节点列表 `node_kind · node_ref`、节点状态。
6. 页面当前不保证直接展示：
`team_id`、`child_agent_id`、`handoff_task_id`。这类字段需走审计查询。

---

## 先搭建一个可测团队（页面）

### 0）先创建 3 个智能体（最小可测）

建议最少创建 3 个（1 主 + 2 子）：

1. `tl-planner-demo`（主智能体）
2. `child-retriever-demo`（子智能体：检索）
3. `child-executor-demo`（子智能体：执行）

可选第 4 个：

1. `child-reviewer-demo`（子智能体：复核）

创建路径（页面）：

1. 打开 `/agent`（或 `/agent/sessions`）。
2. 左侧智能体栏点击 `+`（新建 Agent）。
3. 在右侧配置面板填写：
   - 名称（name）
   - key（建议与上面一致，便于识别）
   - 状态：`active`
   - 系统提示词（见下方角色模板）
   - 能力（Capability）勾选（见下方）
4. 点击保存；重复以上步骤直到创建完 3~4 个智能体。

能力勾选建议（ConfigPanel 的能力复选框）：

1. `tl-planner-demo`：
`text-generation`、`summarization`、`data-analysis`
2. `child-retriever-demo`：
`web-search`、`summarization`
3. `child-executor-demo`：
`text-generation`、`data-analysis`
4. `child-reviewer-demo`（可选）：
`summarization`、`translation`

系统提示词模板（可直接贴）：

1. TL（planner）：
`你是主调度智能体。先拆解任务，再分配子任务，最后汇总；必须显式说明每一步结果与失败点。`
2. Retriever：
`你只负责检索与事实整理，不下结论；输出结构化证据与来源。`
3. Executor：
`你只负责执行动作与生成可执行步骤；输出操作项、预期结果与回滚点。`
4. Reviewer（可选）：
`你只负责复核前述结果的一致性与风险，给出高/中/低结论及依据。`

### 1）创建团队

1. 打开 `/settings/ai/agent-teams`，点击 `创建团队`。
2. 左侧选择上一步创建的智能体（至少 3 个）。
3. 在右侧成员区将 `tl-planner-demo` 设为 TL（点击 `设为 TL`）。
4. 角色分配：
   - `child-retriever-demo` -> `retriever`
   - `child-executor-demo` -> `executor`
   - `child-reviewer-demo`（若有）-> `reviewer`
5. 团队名建议：`a2a-minimal-demo`，点击 `创建团队`。
6. 回到团队列表，确认该团队状态为 `active`。
7. 点击该团队行的 `进入任务`，进入 `/agent/team-tasks`。

推荐最小角色分配：

1. TL：`tl-planner-demo`（planner）
2. 子智能体 A：`child-retriever-demo`（retriever）
3. 子智能体 B：`child-executor-demo`（executor）
4. 子智能体 C：`child-reviewer-demo`（reviewer，可选）

---

## 用例 A：最小并行协作（验证“确实分工”，推荐先跑）

### 前置条件（最小）

1. 已完成上面的“先搭建一个可测团队”。
2. 当前在 `/agent/team-tasks`，且顶部已选中 `a2a-minimal-demo` 团队。

### 在 session 里怎么提问（可复制）

`请并行完成两件事：1）检索 INC-支付网关-延迟飙高 最近24小时变更；2）给出三条可执行修复建议。最后汇总成一个结论。`

### 页面会有什么结果（通过标准）

1. 助手消息出现“执行过程”卡片。
2. 卡片里出现 `Intent：候选`，数量大于等于 2。
3. 卡片里出现 `Plan：节点`，数量大于等于 2。
4. 节点列表里至少出现 2 条节点，并且状态最终为 `completed` 或 `failed`（不能一直 `running`）。
5. 最终正文同时包含“变更摘要 + 3条建议 + 汇总结论”三部分。
6. 如果某子步骤失败，正文里要出现失败说明，不能假装全成功。

### 页面内补充核验（非必须）

1. 打开 `/settings/ai/skills`。
2. 点击 `按 Team 查看 A2A 审计`。
3. 在抽屉输入 `team_id`（来自团队管理页），可选输入 `handoff_task_id / handoff_trace_id` 缩小范围。
4. 期望能看到多条协作记录，字段含 `team / task / trace / node / protocol / status`。

### 后端核验（API 兜底）

1. 用 `trace_id` 查询：`GET /admin/skills/traces?team_id=<team_id>&limit=20`。
2. 期望至少有 2 条 `protocol_used=agent.agent_handoff`（或节点为 handoff 的记录）。
3. 记录内能看到 `team_id` 与 `handoff_task_id`。

### 失败判定

1. 页面无“执行过程”或只有 1 个节点。
2. 最终只回答检索或只回答建议，缺一块。
3. 子任务失败但最终答复没有任何失败说明。

---

## 用例 B：上下文串联协作（证明“先查再判”）

### 推荐团队配置

1. TL：planner
2. 子智能体 A：`retriever`
3. 子智能体 B：`reviewer`

### 在 session 里怎么提问（可复制）

`先查询 INC-订单服务-连接池耗尽 的变更记录，再根据查询结果输出风险复核结论（高/中/低）和依据。`

### 页面会有什么结果（通过标准）

1. “执行过程”里先出现检索相关节点，再出现复核相关节点。
2. 节点列表至少 2 条，且是串联完成，不是只跑一步。
3. 最终正文包含“风险等级 + 依据”，且依据内容与前面检索结果一致。
4. 结论不能脱离检索内容，若证据不足要明确说明“依据不足”。

### 审计核验（页面看不到时）

1. 优先在 Skills 的 `按 Team 查看 A2A 审计` 抽屉查询。
2. 或 `GET /admin/skills/traces?team_id=<team_id>&limit=50`。
3. 同一轮里应能查到多个 `handoff_task_id` 关联记录。
4. 必须能定位到每个子步骤的 `node_id/node_status`。

### 失败判定

1. 第二步结论与第一步检索内容脱节。
2. 只执行了检索，没有执行复核。
3. 子步骤异常但页面和审计都无法定位失败节点。

---

## 用例 C：插件组合协作（证明“业务闭环”）

### 推荐团队配置

1. TL：planner
2. 子智能体 A：`executor`（工具调用主执行）
3. 子智能体 B：`reviewer`（结果复核）

### 前置条件

1. 相关插件能力已接入并可调用（告警、工单、通知至少两个能力可用）。
2. 若插件未接入，此用例会退化为“计划存在但执行失败”，不算通过。

### 在 session 里怎么提问（可复制）

`拉取当前 P1 告警，自动创建工单，并发送值班通知。最后返回告警数、工单号、通知状态。`

### 页面会有什么结果（通过标准）

1. “执行过程”里节点数大于等于 3。
2. 节点状态呈现完整生命周期（运行到完成/失败）。
3. 最终正文出现 3 个回执字段：`告警数`、`工单号`、`通知状态`。
4. 若某步失败，正文明确“部分成功 + 哪一步失败”。

### 审计核验（页面看不到时）

1. 优先在 Skills 的 `按 Team 查看 A2A 审计` 抽屉查询。
2. 或 `GET /admin/skills/traces?team_id=<team_id>&limit=50`。
3. 应能看到多条不同 `node_id` 的执行记录。
4. 失败场景应有对应 `status/error_summary`，并能对上具体 `handoff_task_id`。

### 失败判定

1. 最终没有结构化回执，只给一段笼统描述。
2. 某子任务失败后整个链路中断，且没有“部分成功”结果。

---

## 统一验收记录模板

1. 执行时间：
2. 团队名称：
3. team_id：
4. TL 名称：
5. 输入指令：
6. 页面可见节点数：
7. 最终状态：`成功 / 部分成功 / 失败`
8. 回执摘要：
9. trace_id（用于后端复盘）：
10. 失败节点与原因（如有）：

---

## 建议顺序

1. 先跑用例 A，确认“最小协作存在”。
2. 再跑用例 B，确认“上下文传递正确”。
3. 最后跑用例 C，确认“插件链路可闭环”。
