
# Agent Metrics and Observability

> 本文档定义 PowerX **Agent 层的可观测性与度量体系**。
> 该规范确保所有 Agent、Orchestrator、Adaptor、Workflow、Transport 之间的调用链
> 都能被**完整追踪、量化、审计与可视化**。
>
> 核心目标：**让每一次智能体调用都“有迹可循、可量化、可治理”。**

---

## 1️⃣ 设计目标

| 目标         | 说明                                       |
| ---------- | ---------------------------------------- |
| **统一指标标准** | 所有 Agent 遵循一致的 Metrics 命名与标签体系           |
| **全链路追踪**  | 通过 trace_id 贯穿 Agent → Router → Provider |
| **实时可视化**  | 实时观测执行状态、延迟、错误、资源占用                      |
| **可审计**    | 记录每次调用摘要与授权信息（Scope / Grant）             |
| **轻量集成**   | Agent SDK 自动上报，无需侵入业务逻辑                  |

---

## 2️⃣ 可观测性分层

```
+-----------------------------------------------------------+
|                     PowerX Observability Stack            |
|-----------------------------------------------------------|
|  APM / Metrics / Tracing / Audit / Profiling / Logs       |
+-----------------------------------------------------------+
|  Agent SDK  →  AgentAdaptor  →  Orchestrator  →  Provider |
+-----------------------------------------------------------+
```

| 层级               | 采集内容                 | 输出方式          |
| ---------------- | -------------------- | ------------- |
| **Metrics**      | QPS、延迟、错误率、活跃会话      | Prometheus    |
| **Tracing**      | 调用链、Step 时长、事件流顺序    | OpenTelemetry |
| **Audit Log**    | 授权、调用、Grant 记录       | EventBus / DB |
| **Profiling**    | 内存、CPU、延迟分布          | pprof / eBPF  |
| **Log Sampling** | token/log/state 精简采样 | Loki / ELK    |

---

## 3️⃣ 指标体系总览（Prometheus 格式）

| 名称                              | 类型        | 标签                        | 含义                  |
| ------------------------------- | --------- | ------------------------- | ------------------- |
| `agent_invocations_total`       | counter   | `agent,tenant,capability` | Agent 总调用次数         |
| `agent_invocation_latency_ms`   | histogram | `agent,transport`         | 调用延迟分布              |
| `agent_invocation_errors_total` | counter   | `agent,error_type`        | 调用异常次数              |
| `agent_active_sessions`         | gauge     | `tenant`                  | 当前活跃 Agent 会话       |
| `agent_stream_events_total`     | counter   | `agent,event_type`        | token/log/state 事件数 |
| `agent_cpu_usage_percent`       | gauge     | `agent`                   | CPU 使用率             |
| `agent_memory_mb`               | gauge     | `agent`                   | 内存占用                |
| `agent_grant_validations_total` | counter   | `issuer,subject`          | ToolGrant 验证次数      |
| `agent_grant_denied_total`      | counter   | `issuer,subject`          | ToolGrant 拒绝次数      |
| `agent_proxy_depth_max`         | gauge     | `trace_id`                | 最大代理深度              |
| `agent_a2a_latency_ms`          | histogram | `from,to`                 | Agent→Agent 调用延迟    |
| `agent_retries_total`           | counter   | `agent`                   | 重试次数                |
| `agent_token_throughput_per_s`  | gauge     | `agent`                   | 每秒流式 token 产出       |
| `agent_error_rate_percent`      | gauge     | `agent`                   | 错误率 (%)             |

---

## 4️⃣ 追踪（Tracing）

### 4.1 Trace 结构

每个 Agent 调用自动生成 OpenTelemetry Span：

```
Trace: trc_3b2fa
├── Agent:A1 [invoke goal="客户分析"]
│   ├── Router [select capability="crm.lead.fetch"]
│   │   └── Transport:grpc
│   ├── AgentAdaptor [A2A -> agent:crm_helper]
│   │   └── Agent:B1 [execute skill="crm.lead.fetch"]
│   │       ├── Provider:gRPC
│   │       └── EventBus:publish
│   └── RealtimeGateway:stream
└── Done
```

### 4.2 Span 标签建议

| 标签               | 示例               | 含义          |               |      |
| ---------------- | ---------------- | ----------- | ------------- | ---- |
| `tenant.id`      | `t001`           | 租户标识        |               |      |
| `agent.name`     | `sales_copilot`  | 当前 Agent    |               |      |
| `target.agent`   | `crm_helper`     | 被调用 Agent   |               |      |
| `capability.id`  | `crm.lead.fetch` | 调用的能力       |               |      |
| `transport.type` | `grpc`           | 调用通道        |               |      |
| `toolgrant.id`   | `grant_3abf9`    | 授权 ID       |               |      |
| `trace.id`       | `trc_3b2fa`      | 全局链路 ID     |               |      |
| `span.kind`      | `client/server`  | Span 类型     |               |      |
| `status.code`    | `OK              | ERROR`      | 执行状态          |      |
| `error.type`     | `timeout         | unreachable | grant_denied` | 错误分类 |

---

## 5️⃣ 日志与事件（Event Logging）

所有 Agent 都需输出结构化 JSON 日志：

```json
{
  "ts": "2025-10-12T16:22:31Z",
  "level": "info",
  "trace_id": "trc_3b2fa",
  "agent": "crm_helper",
  "event": "token",
  "data": { "text": "生成客户报告中..." },
  "seq": 42
}
```

> 日志通过 SDK 写入 EventBus（Topic: `agent:<id>:event`），
> 并在 Gateway 层实时推送到前端或监控平台。

---

## 6️⃣ 审计记录（Audit）

每次 Agent 调用都要写入审计表：

| 字段             | 示例                     | 说明           |
| -------------- | ---------------------- | ------------ |
| `audit_id`     | `aud_99a1`             | 唯一 ID        |
| `trace_id`     | `trc_3b2fa`            | 调用链 ID       |
| `tenant_id`    | `t001`                 | 租户           |
| `actor_id`     | `u101`                 | 操作人          |
| `caller_agent` | `sales_copilot`        | 调用方          |
| `target_agent` | `crm_helper`           | 被调用方         |
| `capability`   | `crm.lead.fetch`       | 调用能力         |
| `transport`    | `grpc`                 | 协议类型         |
| `duration_ms`  | `421`                  | 调用耗时         |
| `grant_id`     | `grant_3abf9`          | ToolGrant ID |
| `status`       | `success`              | 结果状态         |
| `created_at`   | `2025-10-12T16:20:03Z` | 记录时间         |

---

## 7️⃣ Profiling 与性能采样

| 指标         | 方法               | 工具                   |
| ---------- | ---------------- | -------------------- |
| CPU        | 每 30s 采样         | pprof / eBPF         |
| 内存         | runtime.MemStats | SDK 内置               |
| goroutines | Go runtime       | Prometheus Exporter  |
| IO         | 系统级 Hook         | optional             |
| 热点分析       | flamegraph       | /debug/pprof/profile |

> SDK 自动暴露 `/metrics` 与 `/debug/pprof` 接口。
> 可统一接入 Grafana、Tempo、Loki。

---

## 8️⃣ EventBus 与 Gateway 集成

| 来源                     | 事件类型                             | 说明      |
| ---------------------- | -------------------------------- | ------- |
| **AgentAdaptor**       | `token`、`log`、`state`、`done`     | 执行事件流   |
| **Security Layer**     | `security.grant_issued`、`revoke` | 授权事件    |
| **Runtime Manager**    | `runtime.health`                 | 健康变化    |
| **Orchestrator**       | `workflow.state`                 | 状态变更    |
| **Metrics Aggregator** | `metrics.snapshot`               | 周期性聚合上报 |

所有事件流都以 `trace_id` 为主键，可回放整个智能体执行链。

---

## 9️⃣ 告警与健康策略

| 触发条件                  | 告警类型  | 动作           |
| --------------------- | ----- | ------------ |
| 调用延迟 > P95            | 性能告警  | 降级 transport |
| 错误率 > 5%              | 稳定性告警 | 重启 Agent     |
| ToolGrant 校验失败 > 10 次 | 安全告警  | 阻断代理调用       |
| MCP 会话掉线 > 60s        | 可用性告警 | 自动重连         |
| 内存 > 1GB 或 CPU > 90%  | 资源告警  | 触发限流或重启      |

> 告警通过 Webhook 或 EventBus 发送到监控系统：
> `Topic: system.alerts.agent`.

---

## 🔟 可视化建议（Grafana Dashboard 模板）

| 面板             | 指标                                           | 可视化形式     |
| -------------- | -------------------------------------------- | --------- |
| 调用趋势           | `agent_invocations_total`                    | 折线图       |
| 调用延迟分布         | `agent_invocation_latency_ms`                | 直方图       |
| 调用错误率          | `agent_error_rate_percent`                   | 饼图        |
| 活跃会话数          | `agent_active_sessions`                      | 实时计数      |
| token 吞吐量      | `agent_token_throughput_per_s`               | 时间序列      |
| CPU/内存         | `agent_cpu_usage_percent`, `agent_memory_mb` | 堆叠图       |
| ToolGrant 使用情况 | `agent_grant_validations_total`              | 表格 + 进度条  |
| Trace 查看       | `OpenTelemetry Tempo`                        | Trace 瀑布图 |

---

## 11️⃣ 与其他模块关系

| 模块                         | 交互                  |
| -------------------------- | ------------------- |
| **AgentAdaptor**           | 上报调用与流式指标           |
| **Security Layer**         | 输出授权与审计事件           |
| **Orchestrator**           | 聚合各 Step 的延迟与 Trace |
| **EventBus / Gateway**     | 推送实时状态与 token 流     |
| **Metrics Layer**          | 收集统一指标数据            |
| **Grafana / Tempo / Loki** | 展示与追踪可视化            |

---

## ✅ 一句话总结

> **Agent Metrics & Observability = 智能体的“黑匣子 + 实时监控仪表盘”。**
> 它让每个 Agent 的行为、性能、调用链、授权、资源都清晰透明，
> 并能实时发现异常、回放全链路、量化系统表现，为 PowerX 的 A2A 编排提供可信观测基础。
