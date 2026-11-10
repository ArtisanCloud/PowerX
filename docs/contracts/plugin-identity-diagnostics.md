# 插件请求身份拒绝日志

## 目的与范围

宿主身份接口成功，不代表插件业务请求一定通过。Core 为插件签发请求身份票据时，还会读取服务端登录会话、用户、租户、成员和插件状态。此前这一阶段失败会返回错误，但没有记录具体拒绝原因。

新增结构化事件 `iam_request_identity_rejected`，覆盖 HTTP 身份票据签发、解析和 WebSocket 身份复核。每次失败只记录一条拒绝事件，沿用现有 Logger 输出通道；正式站启用 Loki 后可直接按事件、Trace ID 和会话指纹搜索，无需启用 HTTP 请求体调试。

这次只增加诊断信息，公开 HTTP 状态、业务错误码、消息和鉴权策略保持原合同。它不补录历史拒绝原因，也不代替登录、用户管理和权限变更的业务审计。

## 字段

| 字段 | 含义 |
|---|---|
| `phase` | `issue`、`resolve` 或 `ws_revalidate` |
| `reason` | 固定的拒绝原因，见下表 |
| `http_status`、`error_code` | 对应公开错误合同，帮助区分用户会话、插件服务凭据、权限与依赖故障 |
| `source_request_id`、`source_trace_id` | 原始插件业务请求的关联标识；票据尚未读取时可能为空 |
| `trace_id`、`correlation_id` | 当前 Core 请求的 Trace ID 与审计关联 ID |
| `plugin_id`、`tenant_uuid`、`user_uuid`、`member_uuid`、`is_root` | 校验对象的身份；缺失或格式无效时相应字段为空 |
| `session_ref` | 原会话 JTI 的完整 SHA-256 指纹；不会记录原始 JTI，可跨请求关联同一会话 |
| `method`、`path`、`transport` | 原始请求目标；路径去除 query/hash，并限制长度 |
| `checked_at`、`user_access_expires_at` | 校验时间和访问 Token 到期时间，UTC RFC3339 格式 |
| `session_expires_at`、`session_revoked_at` | 服务端会话到期／撤销时间，仅在相关拒绝分支有数据时记录 |
| `session_user_matches`、`session_tenant_matches`、`session_member_matches` | 会话与当前请求身份的逐项比较 |
| `proof_expires_at` | 已读取身份票据／WS 句柄的到期时间 |

不记录访问 Token、Refresh Token、API Key、密钥、身份票据、WS 句柄、请求体或依赖的原始错误文本。诊断原因仅进入服务端日志，不添加到公开错误响应的 details 或 message。

## 常见原因与处理方向

| `reason` | 确认的问题 |
|---|---|
| `user_access_expired` | 访问 Token 已到期；查看 `user_access_expires_at` 与 `checked_at` |
| `session_expired` | 数据库登录会话已到期；即使访问 Token 仍有效也会拒绝 |
| `session_revoked` | 登录会话已被撤销；`session_revoked_at` 给出撤销时间，操作者仍需业务审计确认 |
| `session_not_found` | 当前 JTI 找不到有效会话记录；不要直接解释成自然过期 |
| `session_identity_mismatch` | 会话的用户、租户或成员与请求不符；查看逐项比较字段 |
| `session_id_missing`、`user_access_expiry_missing` | 访问凭据缺少会话标识或有效期 |
| `user_issuer_mismatch`、`user_audience_mismatch`、`user_scope_invalid` | 用户凭据的发行方、受众或 scope 与合同不符 |
| `request_identity_fields_invalid` | 身份／请求必要字段缺失或无效；布尔字段给出具体缺项 |
| `user_state_changed`、`member_disabled`、`tenant_disabled` | 用户、成员或租户状态变化，用户原因额外包含状态及 Root 标志比较 |
| `caller_credential_rejected`、`plugin_owner_rejected` | 解析票据的服务调用者凭据、授权或插件归属检查失败；配合 `error_code` 定位，不能当作用户 Token 过期 |
| `proof_required`、`proof_format_invalid`、`proof_not_found`、`proof_expired` | 票据缺失、格式错误、存储未命中或已到期 |
| `ws_handle_required`、`ws_handle_format_invalid`、`ws_handle_not_found`、`ws_handle_expired` | WS 复核句柄对应故障 |
| `session_lookup_failed`、`proof_store_read_failed`、`ticket_store_failed` | 数据库／缓存依赖读取或写入失败；对应 503，不能解释为会话无效 |
| `request_permission_denied`、`permission_snapshot_changed` | 当前请求权限不足或复核期间权限版本改变 |

还有用户／租户／成员／插件查询失败、票据解码失败、管理员检查失败等分支，均使用独立原因值。

## 查询和验收

在监控中心 Logs / Trace 选择实际运行的日志源。正式站当前使用 Loki；先按时间段及原请求 `source_trace_id` 查询，再使用 `session_ref` 关联同一旧会话的其他失败。日志已经存在于普通运行通道，不需要修改日志级别为 debug，也不需要开启请求体日志。

可在 Loki 使用以下查询，时间范围由请求发生时间指定：

```logql
{service="powerx-backend",env="prod"} |= "iam_request_identity_rejected" | json | reason="session_expired"
```

测试必须覆盖 access 仍有效但服务端会话已到期、撤销、会话缺失、身份不匹配、依赖失败、HTTP／WS 复核、单次失败单条事件及敏感内容不落日志，并确认公开状态码和错误码不变。

部署需发布包含此修改的 Core 后端并由操作者重启。无需数据库迁移、能力 seed 或插件升级。未部署时，历史与现有正式进程不会自动产生新事件。
