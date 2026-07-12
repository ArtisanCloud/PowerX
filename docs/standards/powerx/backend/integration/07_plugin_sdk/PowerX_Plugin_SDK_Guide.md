# PowerX Plugin SDK Guide

> 本文档定义 PowerX 插件侧的 SDK 设计规范、启动流程、能力注册、通信机制与安全约束。
>
> SDK 是插件与 PowerX 基座之间的唯一通信层：
>
> * 负责注册能力（capabilities）
> * 建立与 PowerX MCP Gateway 的连接
> * 处理能力调用（invoke/stream）
> * 提供安全认证、心跳与日志上报
>
> 同时**预留 AgentAdaptor / A2A 接口占位**，便于未来扩展。

---

## 1. 设计目标

| 目标         | 描述                            |
| ---------- | ----------------------------- |
| **快速集成**   | 插件通过 SDK 接入，无需理解底层通信协议        |
| **多协议支持**  | SDK 内置 gRPC / HTTP / MCP 三种模式 |
| **统一注册机制** | 插件启动即自动注册能力到 PowerX           |
| **上下文安全**  | 自动注入 tenant / actor / trace   |
| **可观测性**   | 日志、指标、心跳、健康状态自动上报             |
| **A2A 预留** | 插件可以暴露 Agent 类型能力（未来可选启用）     |

---

## 2. 插件生命周期概览

```
┌───────────────────────────────┐
│  Plugin 启动 (Main.go)        │
│    ↓                          │
│  1. 读取 plugin.yaml           │
│  2. 初始化 SDK Client          │
│  3. 注册能力到 PowerX Gateway │
│  4. 启动本地服务 (HTTP/gRPC)   │
│  5. 保持心跳，监听调用         │
│  6. 安全下线 / 清理资源        │
└───────────────────────────────┘
```

---

## 3. plugin.yaml 模板

```yaml
id: com.powerx.plugin.demo.hello
name: Hello Plugin
version: 1.0.0
author: Matrix-X
entrypoint:
  backend:
    transport: grpc
    port: 40001
    start: ./backend/hello-server
capabilities:
  - id: hello.say_hello
    name: Say Hello
    transport: grpc
    description: "返回一条问候消息"
    agent_usable: true
    risk_level: low
    permission_code: hello.greeting:execute
    default_role_grants:
      - role_user
    input_schema:
      type: object
      properties:
        name: { type: string }
    output_schema:
      type: object
      properties:
        message: { type: string }
permissions:
  capability_permissions:
    - capability_id: hello.say_hello
      permission_code: hello.greeting:execute
      title: Say Hello
      agent_usable: true
      risk_level: low
```

`permission_code` 是 Agent 授权和用户 IAM 交集计算的强制字段，格式固定为 `module.resource:action`。SDK/CLI 在注册 capability 时必须校验该字段；缺失或格式错误时，PowerX Registry 不得将该 capability 暴露为可授予 Agent 的能力。

插件 capability 同步到 PowerX Core 后，Core 会把 `permission_code` 注册为租户 IAM permission，并默认授予 `role_owner`、`role_admin`。如果能力面向普通成员使用，插件必须在 capability descriptor 或 manifest 中显式声明 `default_role_grants: [role_user]`；否则普通成员即使能访问 Agent，也会在 Agent 与用户权限交集计算时被拒绝。允许的默认角色码只有 `role_owner`、`role_admin`、`role_user`、`role_readonly`、`role_vendor`，非法值必须注册失败。

---

## 4. SDK 结构概览

```
PowerXPluginSDK
├── core/
│   ├── client.go        # 连接 MCP Gateway
│   ├── registry.go      # 注册能力
│   ├── heartbeat.go     # 心跳机制
│   ├── auth.go          # Token 与租户上下文
│   ├── stream.go        # 流式通信（SSE/GRPC Stream）
│   └── logger.go        # 日志与事件上报
│
├── adapters/
│   ├── grpc_server.go   # gRPC 适配器
│   ├── http_server.go   # HTTP 适配器
│   ├── mcp_client.go    # MCP 客户端
│   └── agent_adaptor.go # (A2A 占位)
│
└── examples/
    ├── hello_world/
    └── ai_text_generator/
```

---

## 5. 插件启动流程

### 5.1 初始化

```go
sdk := plugin.NewSDK(plugin.Config{
  PluginID: "com.powerx.plugin.demo.hello",
  Gateway:  "http://127.0.0.1:8077/mcp",
  Token:    os.Getenv("POWERX_PLUGIN_TOKEN"),
})
```

### 5.2 注册能力

```go
sdk.RegisterCapabilities([]plugin.Capability{
  {
    ID:          "hello.say_hello",
    Name:        "Say Hello",
    Description: "返回一条问候语",
    Transport:   plugin.TransportGRPC,
  },
})
```

### 5.3 启动本地服务

```go
sdk.StartGRPCServer(":40001", func(req *HelloRequest) (*HelloReply, error) {
  return &HelloReply{Message: fmt.Sprintf("Hello, %s!", req.Name)}, nil
})
```

### 5.4 建立 MCP 会话

SDK 自动执行：

```
→ POST /mcp/v1/connect
→ POST /mcp/v1/capabilities
→ 开启 stream 通道
```

### 5.5 心跳与健康上报

* 每 30s 调用 `/mcp/v1/health`
* 若 3 次失败 → SDK 重连
* 若无法恢复 → 标记 `unhealthy`

---

## 6. 能力注册机制

### 6.1 注册流程

```
Plugin SDK → PowerX MCP Gateway → Registry
```

1. 插件读取本地 `plugin.yaml`
2. SDK 组装能力清单（capability list）
3. 调用 Gateway 的 `/mcp/v1/capabilities`
4. Registry 持久化并可供 Router 调用

### 6.2 注册负载

```json
{
  "plugin_id": "com.powerx.plugin.demo.hello",
  "capabilities": [
    {
      "id": "hello.say_hello",
      "transport": "grpc",
      "endpoint": "grpc://127.0.0.1:40001",
      "annotations": {
        "permission_code": "hello.greeting:execute",
        "permission_codes": ["hello.greeting:execute"],
        "agent_usable": true,
        "risk_level": "low"
      }
    }
  ],
  "token": "JWT_TOKEN"
}
```

### 6.3 Agent 授权相关 Manifest 字段

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| `capabilities[].id` | 是 | 稳定 capability ID，不作为权限码使用 |
| `capabilities[].permission_code` | 是 | IAM 权限码，必须为 `module.resource:action` |
| `capabilities[].default_role_grants` | 否 | 除 `role_owner` / `role_admin` 外额外授予的租户默认角色，例如普通成员能力写 `role_user` |
| `capabilities[].agent_usable` | 是 | 是否允许 Agent 被授予并调用该 capability |
| `capabilities[].risk_level` | 是 | 风险等级，建议 `low`、`medium`、`high`、`critical` |
| `permissions.capability_permissions[]` | 是 | 插件声明的 capability 与权限码映射清单 |

Manifest 中不得只声明自由文本 scope、自然语言描述或旧式 capability allowlist。无法映射到结构化 `permission_code` 的能力必须在注册阶段失败。

插件 descriptor 也可以把默认角色授权写在顶层、`security.default_role_grants` 或 `rbac.default_role_grants`。这些字段是授权契约，不是 UI 建议；同步后会直接影响 Agent 运行时的用户权限交集。

---

## 7. 能力调用响应模型

### 7.1 请求结构

```json
{
  "capability": "hello.say_hello",
  "input": { "name": "Matrix" },
  "context": {
    "tenant_id": "t001",
    "actor_id": "u101",
    "trace_id": "trc_abc123"
  },
  "stream": false
}
```

### 7.2 响应结构

```json
{
  "status": "ok",
  "output": {
    "message": "Hello, Matrix!"
  }
}
```

### 7.3 流式响应（SSE / gRPC Stream）

```
event: token
data: {"text": "Hello"}
event: token
data: {"text": ", Matrix!"}
event: done
```

---

## 8. 插件安全机制

| 安全层级     | 策略                        |
| -------- | ------------------------- |
| Token 校验 | 所有请求需带 JWT Token          |
| 租户隔离     | SDK 自动注入 tenant_id        |
| 权限范围     | 插件仅可调用注册的 capability      |
| 文件系统     | 仅访问自身工作目录                 |
| 网络访问     | 禁止外部直连，所有出站经 PowerX Proxy |
| 内存保护     | SDK 内建 panic recovery     |
| 日志脱敏     | 自动过滤敏感字段（如 phone/email）   |

---

## 9. 心跳与健康检查

### 9.1 心跳结构

```json
{
  "plugin_id": "com.powerx.plugin.demo.hello",
  "timestamp": "2025-10-12T11:00:00Z",
  "metrics": {
    "active_threads": 12,
    "cpu": 3.2,
    "mem_mb": 52
  }
}
```

### 9.2 SDK 行为

* 自动每 30 秒发送；
* 可自定义上报周期；
* 当心跳失败超过 3 次 → SDK 触发重连；
* 当检测到 Gateway 无响应 → fallback 到本地缓存队列。

---

## 10. 插件端日志与指标

| 指标                           | 含义      |
| ---------------------------- | ------- |
| `plugin_invocations_total`   | 插件被调用次数 |
| `plugin_latency_seconds`     | 平均响应延迟  |
| `plugin_errors_total`        | 错误次数    |
| `plugin_stream_chunks_total` | 流式输出分片数 |
| `plugin_reconnects_total`    | 重连次数    |

> SDK 自动暴露 `/metrics`（Prometheus 格式）

---

## 11. 开发模式与调试

| 模式                  | 启动方式                  | 特点                         |
| ------------------- | --------------------- | -------------------------- |
| **Local**           | `sdk.RunLocal()`      | 本地直连（无注册）                  |
| **Debug (Reflect)** | `sdk.EnableReflect()` | 可与 `grpcurl` / Insomnia 调试 |
| **Connected**       | 默认模式                  | 自动注册 + 心跳 + 流式通信           |
| **Offline**         | `sdk.RunOffline()`    | 不连接 PowerX，仅本地测试接口         |

---

## 12. 插件目录结构推荐

```
my-plugin/
├── plugin.yaml
├── backend/
│   ├── main.go
│   └── handlers/
│       └── hello.go
├── sdk/
│   └── vendor/powerx-sdk-go/
├── Dockerfile
└── Makefile
```

---

## 13. 协议适配层（Adaptor 绑定）

| Transport     | SDK 绑定                      |
| ------------- | --------------------------- |
| MCP           | 默认连接 Gateway                |
| gRPC          | 内置 gRPC Server 封装           |
| HTTP          | 内置 HTTP Router 封装           |
| Webhook       | SDK 提供注册 API `/hooks`       |
| **Agent（占位）** | 暂不启用，但已定义 `AgentAdaptor` 接口 |

---

## 14. **AgentAdaptor 占位接口**

> 未来用于插件以“Agent 模式”暴露自身能力。

### 14.1 接口定义（预留）

```go
type AgentAdaptor interface {
  RegisterAgent(agent *AgentDefinition) error
  HandleMessage(ctx context.Context, msg *AgentMessage) (*AgentResponse, error)
  Stream(ctx context.Context, ch chan<- *AgentEvent) error
}
```

### 14.2 AgentDefinition（示例）

```go
agent := &plugin.AgentDefinition{
  ID:    "agent.crm.assistant",
  Name:  "CRM 智能助手",
  Skills: []string{"crm.lead.fetch", "crm.customer.update"},
  Description: "自动为销售人员补全客户信息并生成摘要",
}
sdk.RegisterAgent(agent)
```

### 14.3 当前状态

* CoreX 内核已启用 Agent-to-Agent 调度能力；
* Gateway 与 Router 默认关闭对外插件的 `transport=agent` 选路，需平台侧开启 `enable_a2a`；
* SDK 目前仅注册 metadata，并在开关打开后参与实际调用；
* 未开启前，插件仍按 MCP/gRPC/HTTP 流程运行，不受影响。

---

## 15. 插件下线与清理

| 场景       | SDK 行为                    |
| -------- | ------------------------- |
| 进程退出     | 调用 `/mcp/v1/disconnect`   |
| Token 失效 | 尝试刷新；失败则停止注册              |
| Panic    | SDK 捕获后上报异常日志             |
| 升级重启     | 旧 session 清理，新 session 注册 |

---

## 16. 最佳实践

* **尽量使用 SDK 默认的注册与心跳逻辑**；
* **所有能力均应在 plugin.yaml 声明**，SDK 会自动同步；
* **禁止插件自行访问外部网络**，必须通过 PowerX Gateway；
* **日志脱敏 + Trace 统一**（SDK 自动处理）；
* 若需流式输出，务必使用 SDK 提供的 `StreamWriter`；
* 插件应支持优雅退出（`SIGTERM`）。

---

## 17. 插件开发示例（Hello World）

```go
func main() {
  sdk := plugin.NewSDK(plugin.Config{
    PluginID: "com.powerx.plugin.demo.hello",
    Gateway:  "http://127.0.0.1:8077/mcp",
    Token:    os.Getenv("POWERX_PLUGIN_TOKEN"),
  })

  sdk.RegisterCapabilities([]plugin.Capability{{
    ID: "hello.say_hello",
    Transport: plugin.TransportGRPC,
  }})

  sdk.StartGRPCServer(":40001", func(req *HelloRequest) (*HelloReply, error) {
    return &HelloReply{Message: fmt.Sprintf("Hello, %s!", req.Name)}, nil
  })

  sdk.Run()
}
```

---

## 18. 异常与容错策略

| 场景          | SDK 策略                      |
| ----------- | --------------------------- |
| 注册失败        | 立即重试 3 次，指数退避               |
| 心跳中断        | 自动重连；失败后标记 unhealthy        |
| 调用异常        | 返回标准化错误结构 `{code, message}` |
| Token 过期    | 自动刷新 Token                  |
| Gateway 不可用 | 缓存调用并重试                     |
| Panic       | 捕获后上报至日志系统                  |

---

## 19. SDK 更新策略

| 类型   | 更新机制              |
| ---- | ----------------- |
| 轻量更新 | SDK 自动检测新版本（每日一次） |
| 安全补丁 | PowerX 可强制插件升级    |
| 重大版本 | 插件端手动升级 SDK 包     |
| 兼容性  | 新版本 SDK 向后兼容旧协议字段 |

---

## ✅ 一句话总结

> **PowerX Plugin SDK** 是插件与 PowerX 交互的基础通信层。
> 它封装了注册、调用、流式、鉴权、心跳、审计等全套机制，
> 让插件开发者专注于业务逻辑，而不关心传输协议细节。
>
> 当前版本覆盖 MCP/gRPC/HTTP，
> 并已为未来的 **A2A AgentAdaptor** 提前预留接口，无需破坏现有实现。
