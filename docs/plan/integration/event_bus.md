# Event Bus / Task Event 一体化计划（统一规范版）

## 1. 统一口径（唯一事实）

1. **Topic 只表达事件语义**，格式为 `namespace.name`（例如 `knowledge.space.feedback.reprocess`）。
2. **租户只来自 JWT**（`tid/tenant_uuid` claims），不接受 `X-PowerX-Tenant`。
3. **WebSocket 与 Task Event 共用同一 Topic 语义**，仅消费通道不同。
4. **运行时解析链路**：`tenant -> global -> system`。
5. **禁止业务模块私有轮询**，统一走 Event Fabric + TaskDriver。

## 2. 当前状态（2026-02）

### 已统一

- TaskDriver 主路径（Redis 默认，支持 Kafka/RabbitMQ/NATS 适配层）。
- JWT-only 租户识别。
- Challenge Timeout 已改为延迟任务驱动，不走扫库主路径。
- Admin 监控中心已提供 Topic / Queue / DLQ / Replay / 联调入口。

### 进行中（必须收敛）

- 清理旧“租户实例化 topic”思路，收敛到注册制。
- 清理 WS 内存动态注册与 Event Fabric 双轨实现。
- 统一 `event_topics` 注册治理 + ACL 缓存策略（DB 真相源 + Redis 缓存）。

## 3. 架构目标

### 3.1 `event_topics` 注册治理（注册制）

- Topic 必须先注册后使用。
- 注册数据持久化，可审计、可下线、可版本治理。
- 不再要求“每个租户复制一份 Topic 行”。

#### 插件 Topic 声明与落库约束（强制）

1. 插件必须在 `plugin.yaml` 显式声明事件主题（建议 `events.topics[]`，包含 `topic_key + actions`）。
2. 插件安装/启用时必须执行 topic 幂等同步：声明 -> `event_topics`（存在则跳过，不重复创建）。
3. `/internal/ws-bus/grant` 只做授权绑定（ACL），不做 topic 创建。
4. 运行时若 topic 不存在于 `event_topics`，`grant/publish/subscribe/replay` 必须返回明确错误，不做隐式创建。
5. API Key 权限采用固定动作级（publish/subscribe/replay）模板，不按每个 topic 自动生成 permission 记录。

### 3.2 权限模型

- 鉴权主体：JWT（tenant/member/root）。
- 授权模型：角色/主体对 `topic + action`（publish/subscribe/replay）的权限。
- Topic 是否存在与权限是否允许是两步判断，职责分离。

### 3.3 运行时路径

- Publish / Replay / WS Subscribe：
  1) 取 JWT 租户；
  2) 解析 topic key；
  3) 解析 registry（tenant->global->system）；
  4) ACL 判定；
  5) 执行投递/订阅。

## 4. 调试与验收闭环

### Step 1: API 巡检

- `GET /admin/event-fabric/overview`
- `GET /admin/event-fabric/topics`
- `GET /admin/event-fabric/dlq/messages`
- `POST /admin/event-fabric/replay/tasks`
- `POST /internal/ws-bus/grant`

#### `/internal/ws-bus/grant` 统一语义

1. `topics` 必须传语义 key（`namespace.name`）。
2. 租户只来自 JWT（不接受 header/body 覆盖）。
3. 主路径执行：`event_topics` 校验 + ACL 绑定。
4. 返回 `data.mode`：
   - `registry_acl`：主路径生效；
   - `compat_dynamic`：兼容降级路径（仅短期过渡，需尽快消除）。

### Step 2: 联调面板

- 监控中心 `设置 -> 监控中心 -> 事件总线 -> 联调`
- 一键触发后确认：`queued/running/completed`
- 再到 `Queue` 与 `Replay` 页签验证联动

### Step 3: Redis 任务观测（可选）

- 观察 retry / authorization timeout 的队列键变化
- 目标：确认 delay -> queue -> ack 闭环，不出现固定频率扫库

### Step 4: Cron 联调（API 与监控中心一致）

- 列表：`GET /admin/event-fabric/cron/jobs`
- 手动触发：`POST /admin/event-fabric/cron/jobs/{job_id}/run-now`
- 暂停恢复：
  - `POST /admin/event-fabric/cron/jobs/{job_id}/pause`
  - `POST /admin/event-fabric/cron/jobs/{job_id}/resume`
- 联动观察：`GET /admin/event-fabric/task-queue/stats` + DLQ/Replay 接口

> 页面按钮映射关系：
> - “立即执行” = `run-now`
> - “暂停” = `pause`
> - “恢复” = `resume`
> - 列表状态 = `cron/jobs` 返回 `status/next_run_at`


## 5. 后续里程碑

1. **M1（已完成）**：JWT-only + TaskDriver 主路径 + 联调入口。
2. **M2（进行中）**：`event_topics` 统一治理与 WS/Task 双轨收敛。
3. **M3（进行中）**：Cron 调度统一投递已接入（T077~T079），当前补齐测试与运维文档（T083~T084）。

## 6. 文档治理要求

- 所有 Topic 示例必须使用 `namespace.name` 语义 key。
- 文档中不得再出现“请求体手工传 tenant_uuid 决定租户”的描述。
- 修改 WS/Task/Event Fabric 任一协议时，必须同步更新：
  - `docs/guides/async_runtime/event_fabric/integration_playbook.md`
  - `specs/004-eventbus-message-fabric/spec.md`
  - `specs/004-eventbus-message-fabric/quickstart.md`
