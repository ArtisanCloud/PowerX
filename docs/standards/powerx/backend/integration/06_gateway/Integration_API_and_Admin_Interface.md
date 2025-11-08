# Intergration API and Amin Interface

> 本文档定义 PowerX **Integration Framework 的管理与接口规范**，
> 包括控制台可调用的 `/api/v1/admin/*` 管理端 API、指标观测接口、事件订阅与调试入口。
>
> 它覆盖 **Registry、Router、Agent、Workflow、Transport、Security** 等核心域的可运维接口。

---

## 1️⃣ 设计目标

| 目标            | 说明                          |
| ------------- | --------------------------- |
| **统一控制面 API** | 为所有 Integration 模块提供一致的管理入口 |
| **多层观测与诊断**   | 支持注册状态、运行态端点、流式任务与调用链追踪     |
| **安全隔离**      | 仅管理员角色可访问 `/admin` 接口       |
| **事件化架构**     | 提供订阅流式状态变化的 Webhook 与 SSE   |
| **低侵入运维**     | 动态刷新、无感知更新，无需重启 PowerX      |

---

## 2️⃣ 模块范围

| 模块                      | 说明                            |
| ----------------------- | ----------------------------- |
| **Capability Registry** | 能力注册表与端点查看                    |
| **Router**              | 路由策略状态与缓存管理                   |
| **Runtime Endpoint**    | 插件运行态端点与健康检查                  |
| **Workflow**            | 工作流定义、运行与事件追踪                 |
| **Agent / A2A**         | 智能体注册、状态、调用统计                 |
| **Security / Grant**    | Scope、ToolGrant、审计记录          |
| **Metrics / Trace**     | Prometheus + OpenTelemetry 输出 |
| **EventBus / Gateway**  | 实时事件与流式连接信息                   |

---

## 3️⃣ REST API 总览

| Method                       | Path                                          | 模块           | 功能说明            |
| ---------------------------- | --------------------------------------------- | ------------ | --------------- |
| **Registry & Router**        |                                               |              |                 |
| `GET`                        | `/api/v1/admin/capabilities`                  | Registry     | 查询所有注册能力        |
| `GET`                        | `/api/v1/admin/capabilities/{id}`             | Registry     | 查看单个能力详情        |
| `POST`                       | `/api/v1/admin/refresh`                       | Registry     | 手动刷新注册缓存        |
| `GET`                        | `/api/v1/admin/router/cache`                  | Router       | 查看当前路由缓存        |
| `DELETE`                     | `/api/v1/admin/router/cache`                  | Router       | 清空路由缓存          |
| **Runtime Management**       |                                               |              |                 |
| `GET`                        | `/api/v1/admin/runtime/contexts`              | Runtime      | 查看插件运行上下文       |
| `GET`                        | `/api/v1/admin/runtime/endpoints`             | Runtime      | 查看实时端点表         |
| `DELETE`                     | `/api/v1/admin/runtime/endpoints/{id}`        | Runtime      | 下线某端点           |
| **Workflow & Orchestration** |                                               |              |                 |
| `GET`                        | `/api/v1/admin/workflows`                     | Workflow     | 查询所有工作流定义       |
| `GET`                        | `/api/v1/admin/workflow-runs`                 | Workflow     | 当前运行实例          |
| `GET`                        | `/api/v1/admin/workflow-runs/{run_id}`        | Workflow     | 查看运行详情          |
| `GET`                        | `/api/v1/admin/workflow-runs/{run_id}/events` | Workflow     | 回放事件流           |
| **Agent / A2A**              |                                               |              |                 |
| `GET`                        | `/api/v1/admin/agents`                        | AgentManager | 查看已注册 Agent     |
| `GET`                        | `/api/v1/admin/agents/{id}`                   | AgentManager | 查看 Agent 状态与能力  |
| `GET`                        | `/api/v1/admin/agents/{id}/metrics`           | Metrics      | 查看 Agent 指标     |
| `DELETE`                     | `/api/v1/admin/agents/{id}`                   | AgentManager | 停止或注销 Agent     |
| **Security / Governance**    |                                               |              |                 |
| `GET`                        | `/api/v1/admin/security/tool-grants`          | Security     | 查询已签发 Grant     |
| `GET`                        | `/api/v1/admin/security/audit`                | Security     | 查看调用审计记录        |
| `DELETE`                     | `/api/v1/admin/security/tool-grants/{id}`     | Security     | 吊销授权            |
| **EventBus / Gateway**       |                                               |              |                 |
| `GET`                        | `/api/v1/admin/realtime/topics`               | Gateway      | 查看活跃 Topic      |
| `GET`                        | `/api/v1/admin/realtime/connections`          | Gateway      | 查看活跃连接          |
| `DELETE`                     | `/api/v1/admin/realtime/connections/{id}`     | Gateway      | 断开连接            |
| `POST`                       | `/api/v1/admin/realtime/broadcast`            | Gateway      | 系统消息广播          |
| **Metrics / Trace**          |                                               |              |                 |
| `GET`                        | `/api/v1/admin/metrics`                       | Metrics      | Prometheus 指标导出 |
| `GET`                        | `/api/v1/admin/trace/{trace_id}`              | Tracing      | 获取完整 Trace 详情   |
| **Debug / System**           |                                               |              |                 |
| `GET`                        | `/api/v1/admin/healthz`                       | System       | 系统健康检查          |
| `GET`                        | `/api/v1/admin/version`                       | System       | 版本信息与构建时间       |
| `POST`                       | `/api/v1/admin/reload`                        | System       | 重新加载配置          |

---

## 4️⃣ 实时事件订阅（Webhook / SSE）

PowerX 所有运行态变化均可事件化订阅。

| 事件类型                       | 示例 Payload                                                               |
| -------------------------- | ------------------------------------------------------------------------ |
| `registry.updated`         | `{ "capability":"crm.lead.create","provider":"crmplus" }`                |
| `runtime.endpoint.changed` | `{ "plugin":"com.powerx.plugin.crmplus","status":"healthy" }`            |
| `agent.registered`         | `{ "agent":"sales_copilot","skills":["crm.lead.fetch"] }`                |
| `agent.invocation`         | `{ "trace_id":"trc_3b2f","capability":"crm.lead.fetch","duration":128 }` |
| `security.grant_issued`    | `{ "grant_id":"grant_xxx","issuer":"agent:a1","subject":"agent:b2" }`    |
| `workflow.completed`       | `{ "workflow":"lead_summary","status":"success" }`                       |
| `system.alert`             | `{ "severity":"warning","message":"MCP connection lost" }`               |

**订阅接口：**

```
GET /api/v1/admin/events/stream?topic=integration
Header: Authorization: Bearer <admin_token>
```

返回：SSE 实时推送。

---

## 5️⃣ Metrics & Tracing 接口

### 5.1 Metrics Endpoint

```
GET /api/v1/admin/metrics
```

输出 Prometheus 格式：

```
# HELP powerx_agent_invocations_total Agent 调用次数
# TYPE powerx_agent_invocations_total counter
powerx_agent_invocations_total{agent="sales_copilot",tenant="t001"} 543
```

### 5.2 Trace 查询

```
GET /api/v1/admin/trace/{trace_id}
```

返回完整链路 JSON：

```json
{
  "trace_id": "trc_3b2fa",
  "spans": [
    {"name":"AgentAdaptor.Invoke","duration_ms":23},
    {"name":"Router.Select","duration_ms":2},
    {"name":"gRPC.Invoke","duration_ms":18}
  ],
  "status": "success"
}
```

---

## 6️⃣ GraphQL 管理接口（可选）

为控制台 UI 提供可选 GraphQL 查询端点：

```
POST /api/v1/admin/graphql
```

示例查询：

```graphql
query {
  agents {
    id
    name
    health
    invocations
  }
  workflows {
    id
    runs(limit: 5) {
      id
      status
      startedAt
    }
  }
}
```

---

## 7️⃣ 权限与安全

| 规则          | 描述                                 |
| ----------- | ---------------------------------- |
| **仅管理员可访问** | 所有 `/admin` 接口需 `role=admin`       |
| **读写分离**    | 只读接口（GET）对审计员开放；写接口需签名             |
| **签名头**     | `X-Signature` + `X-Timestamp` 防止重放 |
| **多租户隔离**   | 管理员只能查看本租户内的资源                     |
| **审计日志**    | 所有管理操作写入 `security_audit_log`      |

---

## 8️⃣ 集成第三方监控

| 目标系统                   | 集成方式                         |
| ---------------------- | ---------------------------- |
| **Prometheus**         | `/api/v1/admin/metrics` 抓取   |
| **Grafana**            | 使用 Prometheus + Tempo + Loki |
| **Datadog / NewRelic** | OTLP Exporter                |
| **PagerDuty / Feishu** | Webhook on `system.alert`    |
| **ELK Stack**          | 收集 `/var/log/powerx/*.log`   |

---

## 9️⃣ 调试与诊断辅助

| 功能           | 路径                                       | 描述          |
| ------------ | ---------------------------------------- | ----------- |
| Profiling    | `/debug/pprof/*`                         | Go 原生性能分析   |
| Trace 可视化    | `/api/v1/admin/trace/{id}`               | 调用链可视化 JSON |
| Agent SDK 状态 | `/api/v1/admin/agents/{id}/metrics`      | Agent 状态检查  |
| Endpoint 探测  | `/api/v1/admin/runtime/health`           | 健康检测        |
| 流式调试         | `/api/v1/admin/realtime/sse?topic=debug` | 实时事件流       |

---

## 🔟 示例：控制台 Dashboard 视图映射

| 页面    | 后端 API                               | 说明          |
| ----- | ------------------------------------ | ----------- |
| 能力注册表 | `/api/v1/admin/capabilities`         | 查看所有已注册能力   |
| 运行端点  | `/api/v1/admin/runtime/endpoints`    | 实时运行状态      |
| 智能体列表 | `/api/v1/admin/agents`               | 查看 Agent 状态 |
| 工作流运行 | `/api/v1/admin/workflow-runs`        | 实时任务流       |
| 调用链   | `/api/v1/admin/trace/{id}`           | 调用链可视化      |
| 安全与授权 | `/api/v1/admin/security/tool-grants` | 查看/吊销授权     |
| 实时监控  | `/api/v1/admin/realtime/connections` | SSE/WS 连接状态 |
| 系统警报  | `/api/v1/admin/events/stream`        | 接收告警事件流     |

---

## 11️⃣ 扩展与版本化

| 版本     | 特性                        |
| ------ | ------------------------- |
| v1     | REST + Metrics + SSE      |
| v2     | GraphQL 查询与订阅             |
| v3（规划） | WebSocket 控制信道（主动指令）      |
| v4（远期） | 全局 Federated Admin（多节点同步） |

---

## ✅ 一句话总结

> **Integration Admin Interface = PowerX 一体化智能运维控制面。**
> 它为开发者、平台管理员与生态伙伴提供了统一的管理与观测入口，
> 让所有智能体、插件、工作流、能力与安全治理都能被集中监控、动态调度、实时洞察。
