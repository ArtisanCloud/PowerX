# Implementation Plan: Agent Runtime 持久化调度

**Status**: Proposed；所有阶段均待实施。以 [spec.md](./spec.md) 和 [详细设计](../../docs/plan/ai_engineering/agent/agent_runtime_durable_scheduling.md) 为约束。

## 技术边界

Core Agent Runtime 拥有 Run 状态机、依赖图、预算与业务 Outcome；Event Fabric `004` 拥有通用队列驱动的租约、消费组与死信合同；Model Hub `010` 提供模型路由/Profile，Agent 资源池依据真实部署容量发放共享槽位；对象存储保存归档报告；PostgreSQL 保持 Session/Message、业务事实、低频受理与归档引用。生产 Redis 要求 AOF、`noeviction`、备份及恢复演练。`028` 的周期任务调度器不代替 Agent DAG 调度。

## 分阶段实施

1. **冻结合同与基线**：列出现有 Agent SSE、Run logger、DB 快照、Event Fabric Redis driver、Model Profile 和会话 invocation 的实际调用者；定义兼容/迁移窗口和 `run_id` 映射。更新 `007` OpenAPI、`024` Run State 事件 schema、typed capability 声明与消费端。
2. **RunStore**：实现 Redis 状态/事件 CAS、幂等受理、同分片 outbox、受理凭据对账、生产配置门禁及 fail-closed 健康状态。先建立状态和事件一致性，不引入 Worker 双执行。
3. **TaskBus 能力**：在 `004` 的通用队列层扩展 Redis Streams 消费组、租约续期、fencing、重领、延迟重试、死信和去重；用独立验证证明能力声明为 true 后才接 Agent Worker。Agent 不启用通用 DB polling fallback。
4. **DAG 与容量池**：保存不可变计划修订，原子计算就绪任务；按 Run、租户、物理模型池和非模型池容量调度。容量不足进入 `queued`，队列上限/公平性和背压有明确状态。
5. **预算、副作用、恢复**：拆分排队/请求/Run 时限，建立步骤与成本预算；Worker 崩溃和 Redis 故障对账，按业务幂等回执重试或人工核查；租约过期拒绝旧写入。
6. **订阅、迁移、归档**：提交接口快速返回 `run_id`，旧 SSE 入口经同一 Run 订阅而非触发第二套执行；Web Admin/Framework 使用 `event_seq` 续订。对象存储归档可读后清理 Redis 终态细节，历史读按权限取回。
7. **交付门禁**：完成 [quickstart.md](./quickstart.md) 的跨实例、单槽、故障与数据量验收，记录 RPO/RTO、队列吞吐、P95/P99、Redis/DB 写入、归档积压及真实 HTTP/SSE 证据；未通过不标记完成。

## API 与授权

优先延续 `specs/007-integration-gateway-and-mcp/contracts/agent-session.http-openapi.yaml` 的 `/api/v1/tenant/agent/sessions/{session_uuid}/invocations` 受理、读取、取消与事件接口，扩充 Run/Task 状态、序号与恢复语义；新增 API 必须先更新相应 OpenAPI。管理端 Trace 仍由 root 授权。若 Framework 经正式 capability 使用 Core，必须按 [spec.md FR-014](./spec.md) 在同一正式能力合同中声明 Admin 与 typed 服务授权面，不以 Admin REST 权限充当服务态 grant。

## 主要迁移风险

- 旧 SSE 长连接可能仍执行任务：切流时必须按 `run_id`/幂等键单写入、单执行，避免双跑。
- 旧 `pending/running` reducer 不认识 `queued/leased`：版本化事件 schema 与兼容映射必须同批发布。
- Redis cluster 中状态与队列可能跨分片：状态/outbox 原子写后异步投递并对账，不宣称跨分片事务。
- 副作用不能仅靠队列 ACK 判定：实际业务回执与幂等键独立持久化。
