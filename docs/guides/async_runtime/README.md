# PowerX 异步运行时总览（平台级）

> 这份是平台级入口，不属于某一个子域。  
> 目标：统一解释 **Schedule / Event Topic / Task Queue / WebSocket** 的关系与边界。

## 1. 系统边界（先分层）

1. **事件语义层（Event Topic）**  
   - 定义“发生了什么”  
   - 例：`_topic.system.notification`
2. **任务执行层（Task Queue）**  
   - 定义“谁执行、在哪条分片队列执行”  
   - 分片键：`tenant_key + subscriber_id`
3. **实时分发层（WebSocket）**  
   - 定义“谁实时收到结果/通知”
4. **调度层（Schedule）**  
   - 定义“何时触发”
5. **运行记录层（Runs）**  
   - 定义“哪一次触发发生了什么”

结论：`event_fabric` 是这套运行时的当前实现域，不等于全部概念本身。

---

## 2. 一条完整链路（从触发到前端可见）

1. API 或业务模块触发事件/任务
2. Topic 解析 + ACL 校验
3. Task 入队并由 Worker 消费
4. 执行结果再次发布为事件
5. WebSocket 把结果推到页面
6. 历史轨迹写入持久层用于审计和追溯

---

## 3. 文档导航（按问题进入）

1. 我想看平台整体：  
   - `docs/guides/async_runtime/README.md`
2. 我想看 Event Fabric 当前实现：  
   - `docs/guides/async_runtime/event_fabric/README.md`
3. 我想看 Task 机制（队列/重试/历史）：  
   - `docs/guides/async_runtime/task/README.md`
4. 我想看 WebSocket 连接/订阅/推送：  
   - `docs/guides/async_runtime/websocket/README.md`
5. 我想看缓存管理：  
   - `docs/guides/async_runtime/cache/README.md`
6. 我想看 DLQ / Retry：  
   - `docs/guides/async_runtime/dlq_retry/README.md`
7. 我想看调度运行中心：  
   - `docs/guides/async_runtime/scheduler/README.md`
8. 我想看 ACL 与安全：  
   - `docs/guides/async_runtime/acl_security/README.md`
9. 我想看日志与链路追踪：  
   - `docs/guides/async_runtime/observability/README.md`
10. 我想直接联调与运维：  
   - `docs/guides/async_runtime/event_fabric/integration_playbook.md`
11. 我想看命名约束：  
   - `docs/guides/async_runtime/event_fabric/naming_convention.md`
12. 我想看测试脚本与回归：  
   - `docs/guides/async_runtime/testing/README.md`
13. 我想看常见问题：  
   - `docs/guides/async_runtime/faq/README.md`
14. 我想看容量规划（占位）：  
   - `docs/guides/async_runtime/capacity/README.md`
15. 我想看故障恢复（占位）：  
   - `docs/guides/async_runtime/disaster_recovery/README.md`

---

## 4. 当前状态判定（你关心“是否完善”）

是否“完善”不看某个页面，而看这 4 件事是否同时成立：

1. **语义一致**：Topic/Subscriber/Kind 命名统一
2. **链路打通**：Replay 与 Pipeline 都可端到端联调
3. **可观测**：Queue 运行态 + 历史可追溯 + WS 可见
4. **可运维**：调度配置可控制，触发后有运行记录和关联任务可追溯

只要这四项成立，就说明 WebSocket/Event/Task 是一套系统，而不是“只做了 Event”。

---

## 5. PowerXPlugin Framework 接入约定（Host / Standalone）

目标：让 PowerXPlugin 在 framework 层明确“哪些接口可以统一封装、哪些能力要按模式分流”。

### 5.1 模式定义

1. **Host（宿主模式）**
   - 插件运行在 PowerX 宿主上下文
   - 优先复用 Core 的 async_runtime 能力（Event/Task/WS）
   - 插件通用 Scheduler 需要通过 `powerx.scheduler.v1.SchedulerService` 暴露；当前 Event Fabric Cron 只属于底座内部运维能力
2. **Standalone（独立模式）**
   - 插件独立运行
   - 可以复用同一语义契约，但执行底座可为插件本地实现或网关转发

### 5.2 Framework 封装边界（必须遵守）

1. **统一语义层（必须一致）**
   - Topic / Subscriber / Kind 命名
   - 任务状态语义（queued/running/completed/failed 等）
   - 错误语义（unauthorized/not found 等）
2. **模式适配层（允许不同实现）**
   - Host：直接调用 Core async_runtime 入口
   - Standalone：调用插件本地 runtime 或通过网关适配
3. **业务层（禁止感知模式）**
   - 插件业务代码只调用 framework 抽象接口，不直接写 Host/Standalone 分支

### 5.3 当前建议的封装优先级

1. **P0（优先封装）**
   - 事件发布/订阅抽象（Event + WS）
   - 任务提交/查询抽象（Task）
   - 联调与观测上下文字段（trace_id/task_id/topic/subscriber_id）
2. **P1（按项目推进）**
   - Retry / DLQ 管理接口
   - 插件通用 Scheduler facade：CreateJob/UpdateJob/PauseJob/ResumeJob/TriggerJob/GetJob/ListJobs
   - 标准调度触发事件：`powerx.runtime.scheduler.triggered.v1`
3. **P2（规划中）**
   - 调度运行中心：平台内置调度配置、手动触发、暂停、恢复、运行记录、关联任务视图
   - 容量治理策略接口
   - 故障恢复编排接口

### 5.4 文档映射（Framework 开发必读）

1. 命名契约：`docs/guides/async_runtime/event_fabric/naming_convention.md`
2. 机制契约：`docs/guides/async_runtime/task/mechanism.md`
3. 实操契约：`docs/guides/async_runtime/event_fabric/integration_playbook.md`
4. WS 子系统：`docs/guides/async_runtime/websocket/README.md`
5. Task 子系统：`docs/guides/async_runtime/task/README.md`
6. 可观测性：`docs/guides/async_runtime/observability/README.md`
7. 插件通用 Scheduler 规划：`docs/plan/integration/scheduler.md`
8. 调度运行中心：`docs/guides/async_runtime/scheduler/README.md`
