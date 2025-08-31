**（SSE=数据流、WS=控制/交互）**

我把**实现计划**与**要点总结**凝练成一份可直接执行的蓝图（不含实现代码，仅契约与步骤）。

# 一、整体蓝图（如何“跑起来”）

1. **单全局 WS 会话**：前端登录后与后端建立**一条** WebSocket（复用所有 Agent）。
2. **按需建 SSE**：每次发起一次 Agent 运行（run），前端打开**一条 SSE**（仅此 run 的流式事件/文本）。
3. **控制走 WS**：开始/停止/暂停/继续/提交输入/页面交互命令，全部走 WS；WS 回执保证“控制已生效”。
4. **数据走 SSE**：token 增量、工具阶段事件、进度、完成/取消等流经 SSE；关键终止事件（completed/canceled/error）**SSE 与 WS 双通告**（任一路先达即可收尾）。
5. **统一信封**：所有消息（SSE/WS）均含：`tenant_id, agent_id, session_id, run_id, correlation_id, seq, ts, actor_id, source(core|plugin)`。
6. **状态机**：`queued → running → await_input → completed/failed/canceled`；任何重复终止信号幂等处理一次。

# 二、对外契约（必须统一）

## 1) WS 指令（control plane）

* `run.start`（选择 agent/persona/kb、参数）
* `run.cancel | run.pause | run.resume`（带 reason）
* `ui.submit_input`（在 await\_input 阶段）
* `kb.attach | kb.detach`（可选）
* 心跳 `ping/pong` 与 `ack(seq)`（轻量流控）

> WS 所有指令与回执均带 `correlation_id`；回执形如 `*.acked / *.failed`。

## 2) SSE 事件（data plane）

* `run.started`
* `token.delta`（可 50–100ms 合并批）
* `tool.called / tool.result / progress.update`
* `ui.request_input`（提示前端走 WS 提交）
* `run.completed | run.canceled | run.error`

> SSE header 建议携带 `Last-Event-ID` 支持续播；但默认不自动重连，避免重复订阅。

# 三、运行与排队模型（后端）

* **RunManager**：管理 run 生命周期，所有执行上下文（模型流、工具）都绑定可取消的 Context。
* **队列**：`control-queue`（高优先级）与 `token-queue`（可背压/合并）。
* **取消一致性**：收到 `run.cancel` → run 状态置 `canceling` → 级联取消模型流/工具 → 发送 `run.canceled`（SSE+WS）。

# 四、连接与会话管理

* **一个用户/浏览器**：仅一条 WS（多 Tab 通过 BroadcastChannel 复用），多 run 则多条 SSE。
* **重连恢复**：WS 携 `session_id` 与 `last_seq` 以续播回执；SSE 可用 `Last-Event-ID` 简单续播或直接新开。
* **粘性路由**：LB 使用 `session_id` 做 sticky；后端用 Redis/NATS 作为事件背板。

# 五、AI 配置与 seed（无设置不可用）

* **优先级**：System Default → Tenant Default → Agent Override。缺失则后端 4xx/业务码拒绝，前端引导配置。
* **初始化 seed**：安装时落一套最小可用的 `AISetting + 内置Agent + Persona + KB`。
* **健康探测**：启动/定时对 provider/model 做连通与限额检查，避免空跑。

# 六、插件生命周期（与 Agent 的联动）

* 插件 manifest 声明：`agents[]`、`blueprints[]`、`intent_cards[]`、所需能力（tool 权限）。
* **安装**：注册 Agent 与蓝图→重建意图索引→RLS 作用域化→可用。
* **禁用/卸载**：Installation 状态变更→关联 Agent 状态 `disabled`→撤销意图与工具注册→清缓存与索引；禁止删除被引用中的实体。

# 七、意图识别路由

* 建立集中式 **Intent Registry**：结构化卡片（名称、hints、示例、优先级、阈值、租户/场景标签）。
* 路由采用规则 + 向量相似度 + LLM Judge 融合；冲突以优先级/租户策略/用户消歧解决。
* 每次命中输出可审计解释（便于调参）。

# 八、安全与多租户

* 所有消息强制带 `tenant_id`，DB 层走 **RLS**。
* 插件工具调用走 **capability token**（最小权限、TTL、可审计）。
* WS 下发 UI 命令只允许白名单 OPCODE（`open_modal`/`set_progress`/`request_input` 等），JSON Schema 校验、速率限制、幂等。

# 九、性能与拥塞控制（SSE/WS 各自策略）

* **SSE**：token 合并（50–100ms）、段落级退化、代理超时与缓冲调优（见部署建议）。
* **WS**：小型优先级队列（P0=控制、P1=阶段事件、P2=数据回执），高/低水位触发退化；`ack(seq)` + 滑窗避免客户端处理落后。
* **TTI 指标**：用户点击“停止”到后端 `cancel` 生效时间，目标 < 200ms（同机房）。

# 十、观测与审计

* **指标**：按 agent/plugin/model 统计 QPS、时延、token 速率、取消率、重试率、SSE/WS 错误。
* **追踪**：`trace_id / correlation_id` 贯穿；日志对用户输入/工具证据做脱敏存档。
* **回放**：支持按 run\_id 重放（便于问题定位与演示）。

# 十一、前端集成要点（无代码，操作序列）

1. 登录后创建全局 WS（附带 token、tenant\_id、session\_id）。
2. 选择 Agent → 通过 WS 发送 `run.start` → 收到 `run.started` 回执。
3. 立刻新建该 run 的 SSE（URL 带 run\_id）；订阅 `token.delta` 等事件渲染。
4. 用户点“停止”→ WS 发送 `run.cancel`；任一路（SSE 的 `run.canceled` 或 WS 回执）先到即收尾（幂等）。
5. 若 `ui.request_input` 到来，前端展示表单并通过 WS `ui.submit_input` 提交。
6. run 终止后关闭 SSE；WS 保持（供其他 run 与通知）。

# 十二、部署与基础设施建议

* **反向代理**：

    * WS：启用 WebSocket 升级、`proxy_read_timeout` ≥ 300s、开启压缩（permessage-deflate）。
    * SSE：`proxy_buffering off`、`X-Accel-Buffering: no`、`tcp_nodelay on`、读超时 ≥ 300s。
* **水平扩展**：接入层无状态；消息用 Redis/NATS；HPA 以“活跃 run 数/WS 连接数/CPU”综合触发。
* **容错**：客户端断开 SSE 不等于后端停止；以 WS/REST 取消为准。可选增加“最后一个消费者断开→自动取消”的策略开关。

# 十三、验收清单（关键用例）

* 无 AISetting → 调用被明确拒绝（前端获得引导）。
* run 中点击停止：200ms 内停止生效；终止事件 SSE/WS 双达；重复点击不致命（幂等）。
* 插件禁用后：关联 Agent 不可用、意图索引更新、旧会话尝试继续会被拒。
* 多 run 并发：一个 WS、多 SSE 正常工作；取消/暂停只影响目标 run。
* 网络抖动：SSE 降频但不断；WS 重连恢复成功且不重复执行。
* 多租户越权：跨租户访问被 RLS 拦截；工具权限按 capability 控制。

# 十四、里程碑计划（建议 4 个 Sprint）

**M1：契约定稿 & 初始化**

* 信封/事件/指令/状态机/错误码清单
* AISetting 优先级与 seed 数据
* Intent Registry 最小形态与插件 manifest 字段规范

**M2：通讯底座**

* 全局 WS 接入与会话恢复
* SSE 数据流与 run 生命周期、取消一致性
* 指标埋点与基础 Dashboard

**M3：插件与意图**

* 插件安装/禁用联动 Agent 与蓝图
* 意图索引与融合路由（规则+向量）
* 安全：RLS 与 capability token

**M4：稳态与体验**

* 背压/降频策略、TTI<200ms 优化
* UI 指令白名单与 Schema 校验
* 回放/审计/灰度发布与回滚

---
