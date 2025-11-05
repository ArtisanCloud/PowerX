# Plugin Runtime Guide

> PowerX 插件运行态与端口注入说明
>
> 本文档定义 **PowerX 插件在运行期的上下文结构、端口分配、注册机制与安全边界**。
> 适用于通过 PluginManager 加载的所有插件（内嵌式、本地进程、容器化或远程 MCP-Client）。

---

## 1️⃣ 设计目标

| 目标          | 说明                               |
| ----------- | -------------------------------- |
| **统一运行时协议** | 插件启动、端口分配、注册、健康检查流程标准化           |
| **自动发现与注册** | 插件运行时自动向 Registry 汇报自身能力         |
| **多通道通信**   | 支持 HTTP / gRPC / MCP / Local 多协议 |
| **可观测性**    | 提供运行状态、心跳、日志与指标                  |
| **隔离与安全**   | 各插件在独立命名空间、端口与租户下运行              |

---

## 2️⃣ 插件生命周期（Runtime Lifecycle）

| 阶段                 | 行为                                    | 说明                  |
| ------------------ | ------------------------------------- | ------------------- |
| **INSTALL**        | 解析 `plugin.yaml` → 安装依赖 → 注册 Manifest | 写入 DB，创建 schema     |
| **BOOTSTRAP**      | PluginManager 分配端口 → 注入环境变量           | 准备运行环境              |
| **STARTUP**        | 插件启动进程 / 容器 → 暴露端口 → 注册 Runtime       | 创建 `RuntimeContext` |
| **ANNOUNCE**       | 插件向 MCP-Server 反连 → 报告 `capabilities` | 建立 session          |
| **RUNNING**        | 定期心跳 + 健康检测                           | 保持可用状态              |
| **STOP / RESTART** | PluginManager 控制                      | 触发注销与端口回收           |

---

## 3️⃣ 运行时上下文（RuntimeContext）

由 PluginManager 在启动时生成，并通过环境变量注入：

```go
type RuntimeContext struct {
    PluginID      string
    TenantID      string
    Version       string
    GRPCPort      int
    HTTPPort      int
    MCPSessionURL string
    MCPSessionID  string
    Token         string
    RegistryURL   string
    StartedAt     time.Time
}
```

对应注入环境变量：

```
POWERX_PLUGIN_ID=com.powerx.plugin.crmplus
POWERX_TENANT_ID=t001
POWERX_PLUGIN_VERSION=1.0.3
POWERX_ASSIGNED_GRPC_PORT=51043
POWERX_ASSIGNED_HTTP_PORT=51044
POWERX_MCP_SERVER_URL=wss://powerx.internal/mcp
POWERX_MCP_SESSION_ID=sess_a92f
POWERX_MCP_TOKEN=eyJhbGciOi...
POWERX_REGISTRY_URL=http://powerx.internal/api/v1/registry
```

---

## 4️⃣ 端口分配机制

| 类型           | 分配方式                     | 用途            |
| ------------ | ------------------------ | ------------- |
| **HTTP**     | `PluginManager` 动态分配     | BFF/静态资源/控制接口 |
| **gRPC**     | `PluginManager` 动态分配     | 高速 RPC 调用     |
| **MCP**      | 插件主动连接 PowerX MCP-Server | 流式/反向通道       |
| **Local**    | 嵌入式（函数注册）                | 内部调用          |
| **External** | 手动注册                     | 第三方平台连接       |

端口分配范围由配置项控制：

```yaml
runtime:
  http_port_range: [51000, 51999]
  grpc_port_range: [52000, 52999]
  reuse_existing: true
```

---

## 5️⃣ 插件运行时注册流程

```
PluginManager → 分配端口 → 启动插件进程
      │
      ▼
插件读取 ENV，启动自身 HTTP/gRPC 服务
      │
      ▼
插件 → MCP-Server：connect + announce(capabilities)
      │
      ▼
Runtime Manager 汇总端点信息
      │
      ▼
Registry 更新 resolved_endpoints
      │
      ▼
Router 可见该能力
```

---

## 6️⃣ 插件运行目录结构

```
/plugins/
 ├── com.powerx.plugin.crmplus/
 │    ├── plugin.yaml
 │    ├── backend/
 │    │     ├── main.go
 │    │     └── api/
 │    ├── frontend/
 │    │     └── dist/
 │    ├── data/
 │    └── logs/
```

| 目录            | 功能                         |
| ------------- | -------------------------- |
| `plugin.yaml` | 插件元数据定义（ID、版本、依赖、能力）       |
| `backend/`    | 后端服务实现（Go / Rust / Python） |
| `frontend/`   | 前端管理页面（静态构建）               |
| `data/`       | 插件专属持久化数据                  |
| `logs/`       | 运行日志与指标文件                  |

---

## 7️⃣ 运行时服务接口（Health / Capability / Metrics）

### 7.1 健康接口

```
GET /_healthz
→ 200 OK { "status": "healthy", "uptime": 12345 }
```

### 7.2 能力自报接口（可选）

```
GET /_capabilities
→
[
  {"id": "crm.lead.fetch", "transport": "grpc"},
  {"id": "crm.lead.create", "transport": "mcp"}
]
```

### 7.3 Metrics

```
GET /_metrics
# HELP plugin_requests_total 总请求数
plugin_requests_total{status="ok"} 254
```

---

## 8️⃣ 插件运行状态同步

| 来源                  | 更新项                  | 周期        |
| ------------------- | -------------------- | --------- |
| **PluginManager**   | PID、端口、健康、资源         | 10s       |
| **MCP-Server**      | Session、Capabilities | 10s       |
| **Runtime Manager** | TTL 刷新、状态合并          | 15s       |
| **Registry**        | 更新端点表                | 实时        |
| **Router**          | 缓存刷新                 | On-change |

---

## 9️⃣ 日志与可观测性

| 类型   | 路径                            | 描述            |
| ---- | ----------------------------- | ------------- |
| 应用日志 | `/logs/plugin.log`            | 插件内部日志        |
| 启动日志 | `/logs/startup.log`           | 启动阶段输出        |
| 运行指标 | `/logs/metrics.prom`          | Prometheus 格式 |
| 错误事件 | EventBus: `plugin:<id>:error` | 发布异常事件        |

---

## 🔟 安全与隔离策略（简述）

* 插件运行在独立进程（或容器）；
* 每个插件拥有独立端口与环境；
* 所有访问必须通过反向代理；
* 仅允许访问自身租户数据；
* 注册与调用均带签名与 Token；
* 运行时沙箱与 ResourceQuota 由 `infra/plugin/manager` 管理。

> 详见：《05_security/Agent_Security_and_Isolation_Policy.md》

---

## 11️⃣ 故障与恢复策略

| 场景     | 策略                      |
| ------ | ----------------------- |
| 进程崩溃   | 自动重启 + 重注册              |
| 端口占用   | 重试分配新端口                 |
| MCP 断连 | 自动重连                    |
| 注册异常   | 重发注册请求                  |
| 健康探测失败 | 标记 degraded → Router 降级 |
| 配置变更   | 自动 reload，无需重启          |

---

## 12️⃣ 本地调试模式

开发者可在本地通过 CLI 启动插件：

```
powerx plugin run com.powerx.plugin.demo --local
```

自动注入：

```
POWERX_PLUGIN_ID=com.powerx.plugin.demo
POWERX_ASSIGNED_HTTP_PORT=51700
POWERX_MCP_SERVER_URL=ws://localhost:8077/mcp
```

并通过 REST 反代访问：

```
http://localhost:8077/_p/com.powerx.plugin.demo/admin/
```

---

## 13️⃣ 运行态 API（由 Platform 提供）

| Method   | Path                                | 功能      |
| -------- | ----------------------------------- | ------- |
| `GET`    | `/api/v1/admin/runtime/contexts`    | 查看插件上下文 |
| `GET`    | `/api/v1/admin/runtime/endpoints`   | 当前端点表   |
| `POST`   | `/api/v1/admin/plugin/restart/{id}` | 重启插件    |
| `GET`    | `/api/v1/admin/plugin/logs/{id}`    | 实时查看日志  |
| `DELETE` | `/api/v1/admin/plugin/{id}`         | 停止并卸载   |

---

## 14️⃣ 性能指标

| 指标                              | 含义      | 目标     |
| ------------------------------- | ------- | ------ |
| `plugin_start_time_seconds`     | 启动耗时    | ≤ 3s   |
| `plugin_uptime_seconds`         | 运行时长    | -      |
| `plugin_http_latency_ms`        | HTTP 延迟 | ≤ 50ms |
| `plugin_grpc_latency_ms`        | gRPC 延迟 | ≤ 10ms |
| `plugin_heartbeat_lost_total`   | 心跳丢失次数  | ≤ 3 次  |
| `plugin_port_assignments_total` | 分配端口数   | -      |

---

## ✅ 一句话总结

> **Plugin Runtime = PowerX 插件的运行中控层。**
> 它统一管理所有插件的进程、端口、注册、会话与健康状态，
> 让每个插件都能在安全、稳定、可观测的运行态下与 PowerX 能力网络协同工作。
