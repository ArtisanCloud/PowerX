# 04. 日志标签与字段埋点规范

## 1. 目标

统一 PowerX 与插件的日志结构，避免高基数字段污染 Loki label，保证可聚合、可回放、可排障。

## 2. 强制规则

1. Loki `labels` 只放低基数维度：
- `system`
- `service`
- `env`
- `instance`
- `module`
- `level`

2. 高基数字段只放日志正文（JSON fields），不要放 label：
- `trace_id`
- `request_id`
- `tenant_uuid`
- `plugin_id`
- `session_id`
- `message_id`
- `job_id`
- `agent_id`
- `knowledge_base_id`
- `capability_id`
- `user_id`
- `error_detail`

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

## 4. 代码埋点规范（PowerX/插件通用）

### 4.1 基础上下文键

直接在 `context.Context` 放入以下键（字符串键）：
- `trace_id`
- `request_id`
- `tenant_uuid`
- `plugin_id`
- `session_id`
- `message_id`
- `operation`

Logger 会自动提取。

### 4.2 扩展业务字段

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

## 5. 场景示例（tenant + session + message）

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

## 6. 插件对齐建议

插件框架统一输出两层对象：

1. `labels`（低基数，最终映射到 Loki labels）
2. `fields`（高基数，最终进日志正文）

建议插件 SDK 对外接口：
- `log.info(message, { labels, fields })`
- `log.error(message, { labels, fields })`

其中 `labels` 必须限制在 `{system,service,env,instance,module,level}`，其余字段强制降级到 `fields`。

## 7. 必填字段矩阵（执行标准）

| 场景 | 必填 labels | 必填 fields |
|---|---|---|
| HTTP 请求日志 | `system,service,env,instance,module,level` | `trace_id,request_id,tenant_uuid,method,path,status_code,latency_ms` |
| 插件调用日志 | `system,service,env,instance,module,level` | `trace_id,request_id,tenant_uuid,plugin_id,capability_id,action,status_code,latency_ms` |
| 会话消息日志 | `system,service,env,instance,module,level` | `trace_id,tenant_uuid,session_id,message_id,role,event` |
| Agent 执行日志 | `system,service,env,instance,module,level` | `trace_id,tenant_uuid,agent_id,session_id,message_id,phase` |
| Job/调度日志 | `system,service,env,instance,module,level` | `trace_id,tenant_uuid,job_id,job_type,attempt,status,latency_ms` |

## 8. 反例（禁止）

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

## 9. Grafana 查询模板（直接可用）

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
