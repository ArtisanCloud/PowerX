# Capability Registry and Router Design

> 本文档定义 **PowerX Integration Framework** 的核心运行机制：
>
> * 能力注册与同步（Capability Registration）
> * 运行态端点管理（Runtime Endpoint Management）
> * 智能策略路由（Router Policy & Scoring）
> * 健康监控与自动降级（Health & Failover）
>
> 它承接 `Capability_Contract_Spec.md` 与 `Transport_Adapter_Spec.md`，
> 是整个「多协议 + Agent 协作」运行时的**中枢决策层**。

---

## 1️⃣ 设计目标

| 目标        | 描述                                                   |
| --------- | ---------------------------------------------------- |
| **统一真相源** | Registry 维护所有能力、端点与 Provider 元数据。                    |
| **运行态感知** | EndpointResolver 动态生成并缓存端点。                          |
| **策略路由**  | Router 基于延迟、健康、成本、租户、安全策略动态选优。                       |
| **多协议融合** | 支持 MCP / gRPC / HTTP / Agent (A2A) 四类传输通道。           |
| **多租户安全** | 端点绑定租户上下文与 RBAC scope。                               |
| **全链观测**  | Registry、Router、Transport 全链 trace/audit/metrics 可见。 |

---

## 2️⃣ 架构总览

```
+--------------------------------------------------------------+
|                     CoreX / integration                      |
|--------------------------------------------------------------|
|                   Orchestrator / Router                      |
|     (Agent 调度 → 选路 → 调用 Transport → Flow 执行)        |
+--------------------------------------------------------------+
|       Capability Registry & Runtime Endpoint Manager         |
|  ┌──────────────────────────┬──────────────────────────────┐ |
|  | CapabilityRegistry       | EndpointRuntimeResolver      | |
|  |  - Capabilities Meta     |  - Session/Port Context      | |
|  |  - Provider Catalog      |  - Health State Cache        | |
|  |  - Versioning / TTL      |  - Event Watcher (MQ/SSE)    | |
|  └──────────────────────────┴──────────────────────────────┘ |
+--------------------------------------------------------------+
|                Transport Layer (mcp/grpc/http/agent)         |
+--------------------------------------------------------------+
|                 Plugin / Provider Runtime                    |
+--------------------------------------------------------------+
```

---

## 3️⃣ 核心数据模型

### 3.1 Capability Record（持久态）

```yaml
id: crm.lead.create
version: 1.0.0
provider_id: com.powerx.plugin.crmplus
security:
  scope: crm.lead.write
  tenant_scope: t-001
resolved_endpoints:
  - transport: grpc
    uri: grpc://127.0.0.1:51043
    health: healthy
  - transport: mcp
    session: sess_8c21b
    health: healthy
  - transport: agent
    channel: agent://com.powerx.plugin.analytics/session-9d21f
    health: healthy
observability:
  tracing: true
  metrics: true
updated_at: "2025-10-12T05:12:00Z"
```

### 3.2 Runtime Endpoint Table（内存态）

由 **EndpointRuntimeResolver** 实时构建：

```yaml
runtime_endpoints:
  com.powerx.plugin.crmplus:
    grpc:
      addr: 127.0.0.1:51043
      pid: 18332
      health: healthy
      ttl: 30s
    mcp:
      session: sess_8c21b
      connected: true
      last_ping: 2025-10-12T05:10:00Z
    agent:
      channel: agent://com.powerx.plugin.analytics/session-9d21f
      status: active
```

### 3.3 Router Cache

Router 持有热缓存：

* key: capability_id
* value: active endpoints + score snapshot
* 更新：按事件驱动（register/unregister/health_change）

---

## 4️⃣ 注册与运行态同步流程

```
Plugin 启动
    │
    ▼
PluginManager 分配端口 + 启动进程
    │
    ▼
插件 SDK 通过 MCP announce(capabilities)
    │
    ▼
CapabilityRegistry 校验与写入
    │
    ▼
EndpointRuntimeResolver 合并端口/Session → 生成 endpoints
    │
    ▼
Router Cache 更新 → 可立即路由调用
```

### 验证要点

* 能力 ID 唯一；
* Provider Manifest 合法；
* MCP Session 已验证；
* 租户隔离信息写入；
* 初始健康状态 `healthy`。

---

## 5️⃣ 健康与会话监控

| 协议        | 健康机制               | 失效判定                  |
| --------- | ------------------ | --------------------- |
| **MCP**   | WebSocket 心跳 (15s) | 断线 >30s → unreachable |
| **gRPC**  | `/health` 检测       | 连续失败 3 次降级，5 次下线      |
| **HTTP**  | Proxy Ping         | 连续 2 次失败 → degraded   |
| **Agent** | Channel 心跳         | 消息超时 10s → inactive   |

状态流转：

```
healthy ─→ warning ─→ unreachable
   ▲                   │
   └─────(恢复心跳)─────┘
```

---

## 6️⃣ Router 策略引擎

Router 是能力调用的决策器。
它根据上下文与多维指标选择最优端点。

### 6.1 输入上下文

```yaml
{
  capability_id: "crm.lead.create",
  tenant_id: "t-001",
  actor_id: "u-309",
  prefer: "grpc",
  only: "",
  trace_id: "trace-9ab4f"
}
```

### 6.2 策略字段

| 分类  | 字段                                | 含义           |
| --- | --------------------------------- | ------------ |
| 可用性 | `health`, `ttl`                   | 剔除失效端点       |
| 性能  | `latency`, `qps`                  | 低延迟优先        |
| 成本  | `cost_model`                      | 成本感知         |
| 偏好  | `prefer`, `only`, `region`        | 调用上下文约束      |
| 安全  | `scope`, `tenant_scope`           | 权限过滤         |
| 会话  | `session_active`, `channel_alive` | MCP/Agent 状态 |
| 版本  | `version`                         | 兼容或显式匹配      |

---

## 7️⃣ 路由算法（简化伪代码）

```go
func Route(ctx Context, capID string) (*EndpointRecord, error) {
    eps := Registry.GetEndpoints(capID)

    // 1️⃣ 过滤不可用/越权端点
    eps = filter(eps, isHealthy & isScopeAllowed & isTenantAllowed)

    // 2️⃣ 处理 prefer / only 策略
    eps = applyPreference(ctx, eps)

    // 3️⃣ 打分
    scored := rank(eps, calcScore)

    // 4️⃣ 返回最高分端点
    return pickBest(scored)
}

func calcScore(ep EndpointRecord) float64 {
    return 0.4*ep.HealthScore +
           0.3*ep.LatencyScore +
           0.2*ep.CostScore +
           0.1*ep.RegionAffinity
}
```

Router 会持续根据 metrics 动态调整分数。

---

## 8️⃣ 调用生命周期

| 阶段          | 描述                         |
| ----------- | -------------------------- |
| **Prepare** | Orchestrator 构造调用上下文       |
| **Resolve** | Router 选择最优端点              |
| **Invoke**  | Transport 执行请求             |
| **Stream**  | 接收分片结果（可选）                 |
| **Observe** | 上报 trace / metrics / audit |
| **Refresh** | 根据结果调整 Router 缓存评分         |

---

## 9️⃣ 故障与降级策略

| 场景         | Router 行为             |
| ---------- | --------------------- |
| 所有端点失效     | 尝试 fallback transport |
| MCP 断开     | 切换至 gRPC/HTTP         |
| Agent 通道失活 | 等待恢复或返回 async handle  |
| 延迟高        | 动态降权，优选低延迟通道          |
| 版本冲突       | 选择兼容 minor 或回退版本      |

> Router 具备自愈机制：当端点恢复健康，会自动重新纳入候选集。

---

## 🔟 多实例与负载均衡

当同一 Provider 有多个实例：

```yaml
resolved_endpoints:
  - transport: grpc
    uri: grpc://10.0.1.11:50051
  - transport: grpc
    uri: grpc://10.0.1.12:50051
```

Router 策略：

* 轮询或最小延迟；
* 各实例独立心跳；
* TTL 失效自动清理；
* 高可用负载分配。

---

## 11️⃣ 缓存与一致性

| 层级                      | 存储              | 失效机制             |
| ----------------------- | --------------- | ---------------- |
| **Runtime Cache**       | 内存              | TTL + EventWatch |
| **Persistent Registry** | PostgreSQL / KV | 插件重启恢复           |
| **Router Cache**        | LRU / Snapshot  | 定时刷新或事件触发        |
| **Event Watcher**       | SSE / MQ        | 插件上下线即时广播        |

---

## 12️⃣ Metrics 与观测指标

| 指标                            | 标签                   | 说明     |
| ----------------------------- | -------------------- | ------ |
| `registry_capabilities_total` | provider             | 能力注册数量 |
| `endpoint_health_state`       | transport,provider   | 健康分布   |
| `router_latency_seconds`      | capability           | 调用延迟   |
| `router_failures_total`       | capability,transport | 调用失败次数 |
| `router_cache_hits`           |                      | 缓存命中率  |

Trace 链路：

```
Agent → Workflow → Orchestrator → Router → Transport → Provider
```

---

## 13️⃣ 权限与安全控制

| 阶段       | 机制                             |
| -------- | ------------------------------ |
| **注册阶段** | 校验 Provider 签名 + 租户授权          |
| **调用阶段** | Router 检查 actor/scope/tenant   |
| **运行隔离** | 每插件独立 schema/role              |
| **审计记录** | 调用摘要 + trace_id + scope        |
| **数据脱敏** | 按 Capability masking_policy 执行 |

---

## 14️⃣ 故障恢复与热替换

* 插件退出：Registry 标记失效；
* 插件重启：announce 覆盖旧记录；
* MCP 断线重连：恢复 session；
* Router 即时刷新；
* 无需重启 PowerX。

---

## 15️⃣ 控制 API（管理与可视化）

| Method   | Path                                    | 说明     |
| -------- | --------------------------------------- | ------ |
| `GET`    | `/api/v1/integration/capabilities`      | 查询所有能力 |
| `GET`    | `/api/v1/integration/capabilities/{id}` | 能力详情   |
| `GET`    | `/api/v1/integration/runtime/endpoints` | 查看实时端点 |
| `POST`   | `/api/v1/integration/refresh`           | 手动触发刷新 |
| `DELETE` | `/api/v1/integration/endpoints/{id}`    | 强制下线端点 |

---

## 16️⃣ 一图总结运行机制

```
┌────────────────────────────────────────────────────────────┐
│              PowerX Capability Registry & Router            │
│────────────────────────────────────────────────────────────│
│ 1️⃣ 插件注册 → Registry 持久能力 + Resolver 生成端点        │
│ 2️⃣ Health Monitor → 持续探测更新状态                       │
│ 3️⃣ Router 选路 → 过滤 + 打分 + 选优                        │
│ 4️⃣ 调用 → Transport 执行 (mcp/grpc/http/agent)             │
│ 5️⃣ Metrics / Trace / Audit 全链路观测                      │
└────────────────────────────────────────────────────────────┘
```

---

## ✅ 一句话总结

> **Registry 是真相源，Router 是智能决策器。**
> 插件通过 MCP 注册能力，Registry 维护运行态端点，
> Router 在多协议与多智能体之间动态选路、健康感知、智能调度，
> 构建 PowerX 的「运行时可感知、策略自适应」集成核心。
