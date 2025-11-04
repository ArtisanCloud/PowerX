# Runtime Endpoint Management

> 本文档定义 **PowerX CoreX/integration 域** 中的
> **Runtime Endpoint 管理机制**，
> 用于统一维护所有插件与智能体在运行时动态生成的通信端点（HTTP/gRPC/MCP/Agent）。
>
> 它解释：
>
> * 插件启动后如何被分配端口与会话；
> * MCP-Server 如何维护反向连接；
> * RuntimeResolver 如何将这些动态信息同步到 Registry；
> * Router 与 Transport 如何基于这些端点执行智能选路与调用。

---

## 1️⃣ 设计目标

| 目标          | 描述                                                               |
| ----------- | ---------------------------------------------------------------- |
| **统一运行态视图** | CoreX 统一维护所有能力端点（gRPC/MCP/HTTP/Agent）。                           |
| **动态注册与感知** | 插件启动、停止、重连、崩溃等事件自动更新 Registry。                                   |
| **多协议共存**   | 支持 MCP / gRPC / HTTP / Agent(A2A)。                               |
| **健康驱动调度**  | 端点健康状态直接影响 Router 选路。                                            |
| **解耦与自治**   | RuntimeResolver 完全独立于 PluginManager，Router/Adaptor 仅依赖 Registry。 |
| **高可观测性**   | 实时指标、TTL、自愈与可追踪事件流。                                              |

---

## 2️⃣ 架构角色与职责

| 组件                                     | 职责                                                    |
| -------------------------------------- | ----------------------------------------------------- |
| **PluginManager (infra 层)**            | 启动/停止插件进程；分配端口；反代前端；上报 runtime meta。                  |
| **MCP-Server (server 层)**              | 接收插件反连；分配会话 ID；维持心跳与注册事件流。                            |
| **RuntimeResolver (integration 层)**    | 汇总 Port/MCP/Agent 会话；生成 endpoint records；更新 Registry。 |
| **Registry (integration 层)**           | 持久化存储所有能力及运行端点信息。                                     |
| **Router / Transport (integration 层)** | 基于 Registry 进行智能选路与调用。                                |
| **Health Monitor (integration 层)**     | 持续检测端点健康；驱动 TTL、降级与恢复。                                |

---

## 3️⃣ RuntimeContext（运行态上下文）

每个插件启动后，RuntimeResolver 会生成并维护对应的上下文：

```go
type RuntimeContext struct {
    ProviderID     string
    ProcessPID     int
    GRPCPort       int
    HTTPPort       int
    MCPSessionID   string
    AgentChannelID string
    TenantID       string
    Status         string // running|warning|unreachable|stopped
    RegisteredAt   time.Time
    LastHeartbeat  time.Time
}
```

---

## 4️⃣ 启动与注册全链流程

### 4.1 时序图

```
PluginManager → 分配端口 → 启动插件进程
      │
      ▼
插件启动（读取 ENV: GRPC_PORT, HTTP_PORT, MCP_URL）
      │
      ▼
插件 → PowerX MCP-Server: connect + announce(capabilities)
      │
      ▼
RuntimeResolver: 汇总 {grpc_port, http_port, mcp_session, agent_channel}
      │
      ▼
Registry: 更新 resolved_endpoints
      │
      ▼
Router: 即时可见，纳入选路范围
```

### 4.2 环境变量注入（示例）

```
POWERX_PLUGIN_ID=com.powerx.plugin.crmplus
POWERX_ASSIGNED_GRPC_PORT=51043
POWERX_ASSIGNED_HTTP_PORT=51044
POWERX_MCP_SERVER_URL=wss://powerx.internal/mcp
POWERX_MCP_TOKEN=eyJhbGciOiJIUzI1NiIs...
```

---

## 5️⃣ Endpoint 类型与归属

| 类型                 | 维护者                             | 来源             | 用途              |
| ------------------ | ------------------------------- | -------------- | --------------- |
| **HTTP (反代端口)**    | PluginManager                   | 启动分配           | 插件前端 / BFF 页面访问 |
| **gRPC**           | RuntimeResolver + HealthMonitor | 启动时注册 + Health | 内部高速调用          |
| **MCP**            | MCP-Server                      | 插件主动反连         | 反向调用 / 流式执行     |
| **Agent (A2A)**    | AgentManager / RuntimeResolver  | Agent 通道注册     | Agent 间通信       |
| **External (第三方)** | Admin / Connector               | 外部系统接入         | 外部生态调用          |

---

## 6️⃣ Registry 存储格式（运行时合并）

```yaml
resolved_endpoints:
  - transport: grpc
    uri: grpc://127.0.0.1:51043
    service: crm.lead.Service
    method: Create
    health: healthy
  - transport: mcp
    session: mcp://sess_93ba2f
    health: healthy
  - transport: agent
    channel: agent://com.powerx.plugin.analytics/session-8b7c
    health: healthy
```

RuntimeResolver 自动根据 PluginManager / MCP / AgentChannel 事件合并更新。

---

## 7️⃣ 生命周期与事件流

| 阶段       | 事件                              | 行为                                     |
| -------- | ------------------------------- | -------------------------------------- |
| **启动**   | PluginManager 启动进程              | 创建 RuntimeContext、注册端口                 |
| **连接**   | MCP connect / AgentChannel join | 更新会话端点，刷新 Registry                     |
| **运行中**  | Heartbeat / HealthCheck         | 刷新 TTL、维持状态                            |
| **异常断开** | MCP / Channel 断线                | 标记 `warning` / `unreachable`，Router 降级 |
| **重启**   | Plugin 或 Agent 重连               | 替换旧端点、恢复可用                             |
| **停止**   | PluginManager 停止进程              | 清理端点、释放端口                              |

事件均通过 `RuntimeEventBus` 发布给 Router 与 Observability 模块。

---

## 8️⃣ 健康监控与 TTL 自愈机制

| 协议        | 检测方式                   | 阈值       | 状态流转                        |
| --------- | ---------------------- | -------- | --------------------------- |
| **MCP**   | WebSocket ping/pong    | 10s/30s  | healthy→warning→unreachable |
| **gRPC**  | /health RPC            | 3/5 连续失败 | healthy→warning→down        |
| **HTTP**  | /_healthz 或 proxy ping | 3/5 连续失败 | healthy→degraded→down       |
| **Agent** | Channel ping + ack     | 10s      | active→inactive→expired     |

TTL 默认 60s，无心跳自动清理。
插件或 Agent 重连时自动复原，Router 即刻感知。

---

## 9️⃣ 多实例插件与分布式代理

同一 Provider 可有多个实例并行：

```yaml
runtime_endpoints:
  com.powerx.plugin.ai:
    instances:
      - grpc://10.0.0.11:51043
      - grpc://10.0.0.12:51043
      - mcp://sess_91aa22
      - agent://com.powerx.plugin.ai/session-b0aa
```

Router 调度策略：

* 轮询或最小延迟；
* 独立健康检查；
* 实例 TTL 独立；
* 自动下线过期实例。

---

## 🔟 RuntimeResolver 设计

### 10.1 模块职责

| 功能              | 描述                                     |
| --------------- | -------------------------------------- |
| **事件监听**        | 监听 PluginManager、MCP、AgentChannel 事件流。 |
| **上下文聚合**       | 汇总各来源信息生成标准 EndpointRecord。            |
| **Registry 更新** | 调用 RegistryWriter 持久化。                 |
| **通知路由层**       | 向 Router 推送更新事件（on-change 模式）。         |

### 10.2 伪代码

```go
func (r *RuntimeResolver) OnEvent(e RuntimeEvent) {
    switch e.Type {
    case PluginStarted:
        r.updatePort(e.PluginID, e.GRPCPort, e.HTTPPort)
    case MCPConnected:
        r.updateSession(e.PluginID, e.SessionID)
    case AgentChannelJoin:
        r.updateChannel(e.AgentID, e.ChannelURI)
    case HealthChanged:
        r.updateHealth(e.Endpoint, e.Status)
    }
    Registry.Sync(e.PluginID)
    Router.NotifyUpdate(e.PluginID)
}
```

---

## 11️⃣ Metrics 与观测指标

| 指标                               | 标签                 | 描述       |
| -------------------------------- | ------------------ | -------- |
| `runtime_context_total`          | plugin             | 当前运行插件数量 |
| `endpoint_active_total`          | transport          | 激活端点数量   |
| `endpoint_health_state`          | transport,provider | 健康分布     |
| `runtime_registry_updates_total` | plugin             | 更新次数     |
| `runtime_ttl_expired_total`      | plugin             | TTL 清理次数 |

**Trace 链路：**

```
PluginManager → RuntimeResolver → Registry → Router → Transport → Provider
```

---

## 12️⃣ 安全与隔离

* 每端点都绑定 `tenant_id`、`provider_id`；
* 插件只能注册自己能力；
* Router 仅可访问同租户授权端点；
* Agent 通道 ACL 控制 agent-to-agent 调用；
* 所有反代端口经过内置认证网关；
* 无需暴露宿主网络。

---

## 13️⃣ 故障恢复策略

| 故障类型            | 恢复方式                      |
| --------------- | ------------------------- |
| 插件进程崩溃          | PluginManager 自动重启并重新注册端口 |
| MCP 会话断线        | SDK 自动重连，替换旧 session      |
| AgentChannel 超时 | 通知 Router 下线，等待新注册        |
| Registry 异常     | 定期快照 + 恢复同步               |
| Health 探测失败     | Router 自动降级 + 触发自愈循环      |

---

## 14️⃣ 原则与边界

| 原则                      | 说明                                |
| ----------------------- | --------------------------------- |
| **运行态即真相源**             | 所有可调用端点均源自运行态解析结果。                |
| **PluginManager 不参与选路** | 仅负责进程管理与端口分配。                     |
| **Registry 承载真相**       | Router/Transport 仅读不写。            |
| **事件驱动优先**              | 一切更新尽可能通过事件流触发而非轮询。               |
| **多协议一致治理**             | MCP/gRPC/HTTP/Agent 共享同一健康与TTL机制。 |

---

## ✅ 一句话总结

> **Runtime Endpoint Management = PowerX 的运行态通信拓扑核心。**
> 它将来自 PluginManager、MCP、Agent 的多源运行信息聚合入统一真相源，
> 实现端点动态注册、健康驱动、自愈同步，
> 为 Router 和 Transport 提供实时可靠的多协议执行基座。
