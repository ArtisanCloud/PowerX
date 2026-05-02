# 05. Framework 请求链路贯通改造清单（request_id / trace_id）

## 1. 目标

1. 使用单个 `request_id` 可串联 `_p` 网关链路与插件侧日志。
2. 所有关键链路日志统一输出 `request_id`、`trace_id`、`tenant_uuid`、`plugin_id`。
3. 在不增加 Loki 高基数 label 风险的前提下提升排障效率。

## 2. 改造范围

- PowerX backend（framework / gateway）
- `_p/:id/api/*` 代理链路
- 日志采集与 Grafana 查询模板

## 3. 实施清单（按优先级）

### P0：入口与上下文统一

1. HTTP 全局中间件统一注入：
- 读取 `X-Request-ID`，缺失则生成。
- 读取 `X-Trace-Id`/`X-Trace-ID`（或 traceparent），缺失则生成。
- 写入 `context` 与 `gin context`：`request_id`、`trace_id`。
- 响应头必须回写 `X-Request-ID`、`X-Trace-ID`。

2. `_p` 路由处理函数不得二次随机生成 request/trace id。

### P0：_p 关键日志点补齐

以下事件每条都必须打印（最小字段集）：
1. `API-IN`
2. `GATE-ALLOW`
3. `GATE-DENY`
4. `PROXY-OUT`
5. `PROXY-RESP`
6. `PROXY-BACKEND-ERR`
7. `PROXY-TRANSPORT-ERR`

最小字段集：
- `request_id`
- `trace_id`
- `tenant_uuid`
- `plugin_id`
- `method`
- `path/client_path`
- `status`
- `latency_ms`

### P1：上游请求 ID 回填

1. 从插件响应头读取 `X-Request-ID` 记录为 `upstream_request_id`。
2. 若响应头无值，且响应体是 JSON，兜底解析 `request_id`。
3. 在 `PROXY-RESP` 与 `PROXY-BACKEND-ERR` 同时打印 `upstream_request_id`。

### P1：字段命名统一

1. 仅允许字段名：
- `request_id`
- `trace_id`
- `tenant_uuid`
- `plugin_id`

2. 禁止别名：
- `reqId` / `requestId`
- `trace`
- `tid`（可保留业务含义，但不能替代 `tenant_uuid`）
- `plugin`

### P1：插件转发头透传

网关转发到插件时强制透传：
1. `X-Request-ID`
2. `X-Trace-ID`（或系统标准 trace 头）
3. `X-PowerX-Plugin-Id`
4. `tenant_uuid` / `X-PowerX-Tenant`（按既有约定）

### P2：审计日志对齐

`audit_event` 建议补齐：
1. `request_id`
2. `trace_id`
3. `plugin_id`
4. `tenant_uuid`

## 4. Loki 与成本控制（必须遵守）

1. label 白名单：`system,service,env,instance,module,level`。
2. 禁止将 `request_id/trace_id/tenant_uuid/plugin_id/path` 设为 label。
3. 错误明细截断：
- `error_message` <= 512
- `upstream_body_excerpt` <= 1024
4. 采样策略：
- 高频成功 `INFO` 可采样（10%~30%）
- `WARN/ERROR` 不采样

## 5. 验收用例（上线前必过）

1. 成功链路
- 发起一次 `_p` 成功请求。
- 用响应头 `X-Request-ID` 在 Loki/文件日志查询，命中：`API-IN -> GATE-ALLOW -> PROXY-OUT -> PROXY-RESP`。

2. 鉴权拒绝链路
- 触发一次 `GATE-DENY`。
- 同一 `request_id` 能命中 `API-IN` 与 `GATE-DENY`，且带 `deny_reason`。

3. 上游错误链路
- 触发插件返回 4xx/5xx。
- 命中 `PROXY-BACKEND-ERR`，包含 `upstream_request_id` 与 `upstream_status`。

4. 多租户过滤
- 任意日志可按 `tenant_uuid + plugin_id` 过滤。

## 6. 变更窗口与回滚

1. 变更窗口建议先在 dev/staging 验证 24h 再进 prod。
2. 回滚开关建议：
- 可通过配置关闭新增 verbose 字段（保留 request/trace 最小集）。
3. 回滚标准：
- Loki ingestion 明显异常上升。
- 查询延迟显著增加且无法通过采样/截断缓解。

## 7. 发布后观察指标

1. `request_id` 查询命中率（目标：关键链路 > 95%）。
2. Loki ingestion 速率与存储增长曲线。
3. `GATE-DENY` 与 `PROXY-BACKEND-ERR` 的错误占比变化。
4. Grafana 面板加载耗时与查询耗时。
