# Plugin Test and Debug Guide

> PowerX 插件本地调试与反向注册指南
>
> 本文档介绍 **插件开发者如何在本地或测试环境中启动、注册与调试插件**，
> 支持多协议（HTTP/gRPC/MCP）模拟、Registry 注册验证、事件追踪与日志分析。
>
> 目标是让开发者在**不依赖完整 PowerX 部署**的情况下快速验证插件逻辑与能力注册。

---

## 1️⃣ 调试目标

| 目标        | 说明                         |
| --------- | -------------------------- |
| **独立运行**  | 插件可在本地进程中单独启动运行            |
| **自动注册**  | 模拟 PluginManager 注入端口与环境变量 |
| **可观测性**  | 本地查看日志、端点、健康与注册状态          |
| **多协议验证** | 同时测试 HTTP、gRPC、MCP 调用      |
| **断点与追踪** | 支持 IDE 断点调试与 Trace 输出      |

---

## 2️⃣ 调试环境准备

### 2.1 前置依赖

| 组件               | 说明               |
| ---------------- | ---------------- |
| **Go ≥ 1.22**    | 推荐编译与运行插件的语言版本   |
| **PowerX Core**  | 主框架运行（或 mock 版本） |
| **NATS / Redis** | EventBus（可选）     |
| **PostgreSQL**   | Registry 持久层（可选） |
| **PowerX CLI**   | 插件运行命令行工具        |

安装 CLI：

```bash
go install github.com/ArtisanCloud/PowerX/cmd/powerx@latest
```

---

## 3️⃣ 本地启动插件（独立模式）

```bash
powerx plugin run com.powerx.plugin.crmplus --local
```

默认行为：

* 自动生成临时 Registry；
* 启动 gRPC / HTTP；
* 启动 MCP Mock Server；
* 输出注册事件与日志。

### 3.1 示例输出

```
[PLUGIN] Starting plugin: com.powerx.plugin.crmplus
[PLUGIN] HTTP Port: 51701
[PLUGIN] gRPC Port: 52701
[PLUGIN] MCP Connected: sess_local_1a9d
[PLUGIN] Registered capabilities: crm.lead.fetch, crm.lead.create
```

---

## 4️⃣ 手动注入运行时变量

如需绕过 CLI，直接手动运行：

```bash
export POWERX_PLUGIN_ID=com.powerx.plugin.demo
export POWERX_ASSIGNED_HTTP_PORT=51090
export POWERX_ASSIGNED_GRPC_PORT=52090
export POWERX_MCP_SERVER_URL=ws://localhost:8077/mcp
export POWERX_MCP_SESSION_ID=sess_local_demo
export POWERX_MCP_TOKEN=localtesttoken
go run backend/main.go
```

---

## 5️⃣ 调试 HTTP 接口

插件启动后即可直接调用：

```
curl -X GET http://localhost:51090/_healthz
curl -X POST http://localhost:51090/api/demo/echo -d '{"text":"hello"}'
```

PowerX 反向代理路径示例（生产模式）：

```
http://localhost:8077/_p/com.powerx.plugin.demo/api/demo/echo
```

---

## 6️⃣ 调试 gRPC 接口

生成 gRPC Stub：

```bash
grpcurl -plaintext localhost:52090 list
grpcurl -plaintext localhost:52090 describe crm.lead.Service
grpcurl -d '{"id":"L001"}' localhost:52090 crm.lead.Service.Fetch
```

常用调试选项：

| 参数                                   | 说明       |
| ------------------------------------ | -------- |
| `-plaintext`                         | 不启用 TLS  |
| `-H "Authorization: Bearer <token>"` | 测试授权     |
| `-vv`                                | 显示完整流式输出 |

---

## 7️⃣ 调试 MCP（反向连接）

插件端启动 MCP Client（内置于 SDK）后，会尝试连接 MCP Server：

```
[PLUGIN] Connecting MCP Server: ws://localhost:8077/mcp
[PLUGIN] Session ID: sess_8f3a1
[PLUGIN] Announcing 3 capabilities...
```

PowerX 端输出：

```
[MCP] Session connected: plugin=com.powerx.plugin.crmplus
[MCP] Capability registered: crm.lead.fetch
```

### 7.1 测试反向调用

```bash
curl -X POST http://localhost:8077/api/v1/mcp/invoke \
     -d '{"plugin_id":"com.powerx.plugin.crmplus","capability":"crm.lead.fetch"}'
```

---

## 8️⃣ 验证 Registry 注册状态

```bash
curl http://localhost:8077/api/v1/admin/runtime/endpoints | jq
```

输出示例：

```json
{
  "com.powerx.plugin.crmplus": {
    "grpc": {"addr": "127.0.0.1:52090", "health": "healthy"},
    "http": {"addr": "127.0.0.1:51090", "health": "healthy"},
    "mcp": {"session": "sess_8f3a1", "connected": true}
  }
}
```

---

## 9️⃣ 断点调试（VSCode / GoLand）

在 `backend/main.go` 设置断点 → 使用 CLI 启动本地模式：

```
powerx plugin run com.powerx.plugin.demo --local --debug
```

或在 IDE 配置：

* 运行环境：`Run → Edit Configurations`
* 环境变量：同上
* WorkingDir：插件根目录

> SDK 会打印 trace_id 与请求上下文，方便链路分析。

---

## 🔟 调试日志与追踪

| 文件                   | 内容            |
| -------------------- | ------------- |
| `/logs/plugin.log`   | 主运行日志         |
| `/logs/trace.log`    | 跟踪请求链         |
| `/logs/mcp.log`      | MCP 连接与消息     |
| `/logs/health.log`   | 健康状态变化        |
| `/logs/metrics.prom` | Prometheus 指标 |

示例：

```
[TRACE] crm.lead.fetch trace_id=trc_92f duration=35ms
[MCP] session=sess_8f3a1 ping ok
[HEALTH] status=healthy ttl=60s
```

---

## 11️⃣ 模拟健康探测与重启

```
powerx plugin check com.powerx.plugin.demo
powerx plugin restart com.powerx.plugin.demo
```

或直接调用 API：

```
GET  /api/v1/admin/runtime/endpoints
POST /api/v1/admin/plugin/restart/com.powerx.plugin.demo
```

---

## 12️⃣ 事件与日志流调试

本地模拟 EventBus：

```bash
powerx event listen plugin:com.powerx.plugin.demo
```

示例输出：

```
[event] plugin:com.powerx.plugin.demo → log { "msg": "执行成功" }
[event] plugin:com.powerx.plugin.demo → error { "code": 400 }
```

---

## 13️⃣ 常见问题排查

| 问题            | 原因                   | 解决方案                       |
| ------------- | -------------------- | -------------------------- |
| 无法注册端点        | 端口冲突 / 无法访问 Registry | 检查端口范围、网络配置                |
| MCP 连接失败      | 目标 URL 错误            | 检查 `POWERX_MCP_SERVER_URL` |
| 能力未发现         | 未调用 `announce()`     | 调用 SDK 初始化                 |
| 健康状态 degraded | 健康探测未通过              | 检查 `_healthz` 响应           |
| 调试日志无输出       | 无 `--debug` 参数       | 启动时添加 `--debug`            |

---

## 14️⃣ 推荐开发循环

```
修改代码 → 本地启动 (--local)
   ↓
查看 Registry 注册结果
   ↓
调用 HTTP/gRPC/MCP 接口测试
   ↓
事件追踪 → 调试日志
   ↓
调整与重启
```

---

## 15️⃣ 最佳实践

| 场景            | 建议                                                  |
| ------------- | --------------------------------------------------- |
| **本地开发**      | 使用 `--local` 模式，启用 auto-register                    |
| **团队联调**      | 使用共享 Registry（Docker Compose 启动 PowerX Core）        |
| **CI 测试**     | 在 pipeline 中运行 `powerx plugin test`                 |
| **Mock 能力调用** | 用 `powerx mock invoke <capability>` 验证输出            |
| **性能测试**      | `powerx bench plugin <id> --qps 50 --concurrent 10` |

---

## ✅ 一句话总结

> **Plugin Test & Debug = 插件开发者的实验沙箱。**
> 它让插件在本地即可完成启动、注册、反连、调用与链路追踪，
> 无需完整 PowerX 环境即可验证所有能力注册、运行与监控逻辑。
