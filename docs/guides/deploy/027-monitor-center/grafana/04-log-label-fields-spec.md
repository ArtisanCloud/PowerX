# 04. 日志标签与字段规范（执行版）

## 1. 目标

1. 用一个 `request_id` 串起网关与插件链路。
2. 保证 Loki 成本可控，不因高基数字段爆炸。
3. 统一字段命名，避免团队各写各的。

## 2. 日志分级策略（必须）

1. `INFO`：只打最小字段集。
2. `WARN`：最小字段集 + 诊断字段集。
3. `ERROR`：最小字段集 + 诊断字段集 + 错误细节（截断）。

## 3. 字段分层规范

### 3.1 最小字段集（INFO/WARN/ERROR 都必须有）

- `timestamp`
- `level`
- `system`
- `service`
- `env`
- `instance`
- `module`
- `event`（如 `API-IN`、`GATE-DENY`、`PROXY-RESP`）
- `request_id`
- `trace_id`
- `tenant_uuid`
- `plugin_id`
- `method`
- `path`（或 `client_path`）
- `status`
- `latency_ms`

### 3.2 诊断字段集（WARN/ERROR 必须有，INFO 可选）

- `gate_decision`
- `deny_reason`
- `upstream_host`
- `upstream_path`
- `upstream_status`
- `upstream_request_id`
- `correlation_id`
- `user_id`
- `member_id`

### 3.3 错误细节（仅 ERROR）

- `error_code`
- `error_message`（最大 512 字符）
- `transport_error`（最大 512 字符）
- `upstream_body_excerpt`（最大 1024 字符）

## 4. Loki Label 白名单/黑名单（必须）

### 4.1 允许作为 label（低基数）

- `system`
- `service`
- `env`
- `instance`
- `module`
- `level`（可选）

### 4.2 禁止作为 label（高基数）

- `request_id`
- `trace_id`
- `tenant_uuid`
- `plugin_id`
- `user_id`
- `member_id`
- `path`
- `correlation_id`
- `upstream_request_id`
- `session_id`
- `message_id`
- `job_id`

## 5. 命名统一（强制）

1. 只用 `request_id`，禁用 `reqId/requestId`。
2. 只用 `trace_id`，禁用 `trace`。
3. 只用 `tenant_uuid`，禁用 `tid/tenantId`。
4. 只用 `plugin_id`，禁用 `plugin/pluginId`。

## 3. PowerX 侧配置示例

`/etc/powerx/config.yaml`：

```yaml
log:
  loki:
    enable: true
    url: http://127.0.0.1:3100
    labels:
      system: powerx
      service: powerx-backend
      env: prod
      instance: cn-hz-prod-01
      module: runtime
```

## 6. 代码埋点规范（PowerX/插件通用）

### 6.1 基础上下文键

直接在 `context.Context` 放入以下键（字符串键）：
- `trace_id`
- `request_id`
- `tenant_uuid`
- `plugin_id`
- `session_id`
- `message_id`
- `operation`

Logger 会自动提取。

### 6.2 扩展业务字段

通过 `logger.WithLogFields(ctx, map[string]interface{}{...})` 注入。

示例：

```go
ctx = logger.WithLogFields(ctx, map[string]interface{}{
  "session_id": sessionID,
  "message_id": messageID,
  "job_id": jobID,
  "phase": "policy_evaluate",
})
logger.Info(ctx, "plugin invocation accepted")
```

## 7. _p 代理链路必打事件（PowerX framework）

以下事件每条都必须带最小字段集：

1. `API-IN`
2. `GATE-ALLOW`
3. `GATE-DENY`
4. `PROXY-OUT`
5. `PROXY-RESP`
6. `PROXY-BACKEND-ERR`
7. `PROXY-TRANSPORT-ERR`

## 7.1 日志源覆盖范围（统一上下文字段）

除 `_p` 代理链路外，以下日志源同样必须输出并对齐 `request_id/trace_id/plugin_id/tenant_uuid`：

1. `http_request`
2. `audit_event`
3. `wsbus.*`（publish/subscribe/emit/deliver/ack/drop）
4. plugin supervisor 日志
5. 异步 worker（cron/queue/retry/event-fabric）

补充要求：
- `/api/v1/integration/<slug>/...` 必须解析并注入 `plugin_id`（slug -> plugin_id）。
- 关键字段不得只放在 `meta`；必须作为顶层字段可检索。
- 异步链路禁止无条件 `context.Background()` 断链。

## 8. 场景示例（tenant + session + message）

一次会话链路建议最少包含：
- `tenant_uuid`
- `session_id`
- `message_id`
- `trace_id`

查询示例：

```logql
{service="powerx-backend",env="prod"} | json | tenant_uuid="6b5d0240-9920-46da-b707-88200e0f51ea" | session_id="sess_123"
```

```logql
{service="powerx-backend",env="prod"} | json | message_id="msg_456"
```

## 9. 插件对齐建议

插件框架统一输出两层对象：

1. `labels`（低基数，最终映射到 Loki labels）
2. `fields`（高基数，最终进日志正文）

建议插件 SDK 对外接口：
- `log.info(message, { labels, fields })`
- `log.error(message, { labels, fields })`

其中 `labels` 必须限制在 `{system,service,env,instance,module,level}`，其余字段强制降级到 `fields`。

## 10. 必填字段矩阵（执行标准）

| 场景 | 必填 labels | 必填 fields |
|---|---|---|
| HTTP 请求日志 | `system,service,env,instance,module,level` | `trace_id,request_id,tenant_uuid,method,path,status_code,latency_ms` |
| 插件调用日志 | `system,service,env,instance,module,level` | `trace_id,request_id,tenant_uuid,plugin_id,capability_id,action,status_code,latency_ms` |
| 会话消息日志 | `system,service,env,instance,module,level` | `trace_id,tenant_uuid,session_id,message_id,role,event` |
| Agent 执行日志 | `system,service,env,instance,module,level` | `trace_id,tenant_uuid,agent_id,session_id,message_id,phase` |
| Job/调度日志 | `system,service,env,instance,module,level` | `trace_id,tenant_uuid,job_id,job_type,attempt,status,latency_ms` |

## 11. 成本控制（防日志库爆炸）

1. 仅 `WARN/ERROR` 打诊断字段，`INFO` 保持最小集。
2. 高频成功请求可采样（10%~30%，按业务可调）。
3. `WARN/ERROR` 不采样。
4. 健康检查、探针、静态资源日志降噪或抑制。
5. 保留策略建议：
- `INFO`：7~15 天
- `WARN/ERROR`：30~90 天

## 12. 反例（禁止）

1. 把 UUID 放到 label：

```text
labels: {tenant_uuid="6b5d...", session_id="sess_123"}
```

2. 直接写 `context.Background()`：

```go
logger.Info(context.Background(), "job started")
```

3. 用不受控 label key：

```text
labels: {customer_name="...", page_url="..."}
```

## 13. Grafana 查询模板（直接可用）

先按 labels 选流，再按 fields 过滤。

1. HTTP 错误趋势：

```logql
sum by (path, status_code) (
  count_over_time({system="$system",service="$service",env="$env",module="http",level=~"warn|error"} | json [5m])
)
```

2. 租户会话追踪：

```logql
{system="$system",service="$service",env="$env"} | json | tenant_uuid="$tenant_uuid" | session_id="$session_id"
```

3. 插件调用失败：

```logql
{system="$system",service="$service",env="$env",module="plugin",level="error"} | json | plugin_id="$plugin_id"
```

4. Agent 单次消息全链路：

```logql
{system="$system",service="$service",env="$env"} | json | message_id="$message_id"
```

## 14. 验收标准（上线前必须满足）

1. 任意一个 `_p` 请求，能用同一 `request_id` 查询到完整网关链路。
2. 任意日志行可按 `tenant_uuid + plugin_id` 快速过滤。
3. 发布前后 Loki label cardinality 无异常飙升。

## 15. 与当前线上日志格式的对齐说明（重要）

当前你们线上日志同时存在两类：

1. 结构化 JSON 行（可用 `| json` 解析字段）。
2. 文本前缀行（如 `[API-IN]`、`[GATE-DENY]`，字段在正文中，需 `|=`, `| regexp` 过滤）。

因此建议：

1. 看趋势：优先用 label + 文本关键词。
2. 看字段聚合：仅在确认该类日志是 JSON 后再 `| json`。

示例（你当前已验证可用）：

```logql
{system="powerx",service="powerx-backend",env="dev"} |= "no permission rule for this route"
```

```logql
{system="powerx",service="powerx-backend",env="dev"} |= "/api/v1/agent/ai-craft/products"
```

## 16. Dashboard 变量建议（避免空值无结果）

1. `system`：建议 `Custom`，默认值 `powerx`（可不提供 All）。
2. `service/env/instance/module`：`Query` 变量。
3. `level`：`Custom` 变量，值 `debug,info,warn,error`，All value=`.*`（用于正文级别过滤）。
