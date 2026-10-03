# T001 当前调用链与迁移矩阵

**核对日期**：2026-09-23。此文件记录开发起点，不表示目标链路已实现。

| 边界 | 当前消费者/实现 | 031 迁移要求 |
| --- | --- | --- |
| Web Admin Chat | `web-admin/app/composables/agent/useDualChannelConnection.ts` 直接 `GET /agents/stream/sse`，同一请求携带用户输入并消费 `agent_run.*`；中止 fetch 只停止该连接。 | 改为一次幂等提交取得 `run_id`，再订阅事件；同一消息的任务仍汇入一个执行卡片，断线按 `event_seq` 恢复。 |
| Core Agent 执行 | `backend/internal/server/agent/runtime/engine.go` 调用 `Manager.ExecutePlanWithHooks`；`manager_execute.go` 按 stage 使用进程内 `errgroup.SetLimit`。 | 保存不可变 DAG/任务尝试，由 Worker 领取；依赖与资源池共同决定就绪，不由 HTTP 连接生命周期决定执行。 |
| Core Run State | `backend/internal/server/agent/runtime/run_state_events.go` 将执行事件映射为 `agent_run.*`；`backend/pkg/dto/stream_events.go` 仅列当前六种 task 展示状态。 | 版本化增加排队、租约、验证、重试等待和原因字段；状态写入成功后发布事件，不能以日志或 SSE 作为权威。 |
| 服务态会话 | `backend/internal/service/agent_session/invocation.go` 在 DB 插入 `running` invocation 后启动进程内 goroutine；`service_invocation_repo.go` 在 deadline 后将孤儿运行标记失败，不自动恢复。 | 受理记录仍为低频 DB 锚点；Redis RunStore + Worker 接管执行，重启后按租约/回执恢复或明确阻断。切流必须避免 goroutine 和 Worker 双执行。 |
| 服务态事件 | `backend/internal/transport/http/openapi/agent_session/handler.go` 轮询 invocation，只发 `state=1/terminal=2/end=3`，`Last-Event-ID` 仅支持这三个静态位置。 | 扩展为 `run_id + event_seq` 有界续订；历史过期读快照或归档。`007` OpenAPI 当前 wire 合同保留为基线，031 T002/T009 再正式修改。 |
| Framework Go/.NET | Go `framework/backend/go/runtime/powerx/agent/service_sessions.go` 与 .NET `PowerXAgentSessionClient.cs` 使用既有会话 invocation 和 `Last-Event-ID`；Go `run_state.go` 和插件调试页消费现有 `agent_run.*`。 | 同批升级 typed DTO、序号游标与 reducer；不能借 Admin REST 权限代替服务态 capability grant。Framework 仓库是独立 checkout，修改需单独验证。 |
| Event Fabric TaskQueue | `backend/pkg/event_bus/redis_task_driver.go` 采用 Redis list `BRPopLPush`/processing/inflight hash，`SupportsLease=false`、`SupportsConsumerGroup=false`；`backend/internal/app/shared/deps.go` 为通用任务接入可回退路径。 | 004 TaskBus 增补 Streams 消费组、租约/fencing、重领、去重、DLQ；Agent 使用严格模式并禁止 DB fallback。现有驱动不可直接当作 Agent Worker 队列。 |
| 模型容量 | `backend/internal/server/ai/factory/llm/concurrency.go` 的 gate 是进程内 map，键含 tenant/env/provider/model；配置来自 Profile `max_concurrent_requests`。 | 按实际 endpoint/model/实例建立跨 Core、跨租户物理池；Profile 值是策略上限。模型排队时间不计入单次请求超时。该文件当前有其他未提交改动，迁移时须保留并整合。 |
| Trace/历史 | `backend/internal/server/agent/bootstrap/run_logger.go` 与现有 DB 快照、Local File/Loki Trace 用于排障和页面恢复。 | Redis 活动状态 + 对象存储归档为 Run 权威；DB 仅保留业务事实、受理锚点和 locator，日志不是调度账本。 |

## 合同与切流决定

1. 先实现 RunStore 和可验证的通用队列租约能力，再在受控配置下把单一入口交给 Worker；旧 SSE/goroutine 与新 Worker 不得对同一 `run_id` 同时执行。
2. 既有 `/api/v1/tenant/agent/sessions/{session_uuid}/invocations` 保持服务态兼容入口；变更成功响应和事件前先更新 `007` OpenAPI、Framework Go/.NET typed client 与合同测试。Admin Chat 路径单独迁移，并保持授权面分离。
3. `agent_run.*` 继续承载 UI 状态；普通 `final` 是用户可见正文。旧 reducer 只理解 `pending/running/...`，需显式版本映射，不能直接发未知状态导致错误的完成展示。
4. 所有跨 checkout 改动保留各自未提交工作；用配置/合同测试完成灰度与回退，不能通过生产隐式 memory/DB fallback 掩盖 Redis 不可用。
