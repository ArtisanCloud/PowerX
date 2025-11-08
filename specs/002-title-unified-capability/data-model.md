# 数据模型设计 — Unified Capability Contracts & Transport Adapters

## 领域概览

统一能力契约域负责存储并暴露能力（Capability）的结构化定义、版本策略、错误分类以及跨协议的传输偏好。所有调用路径（Agent、插件、HTTP/gRPC 客户端）都将从这里获取权威数据，并由 Router/Adapter 在运行时执行。

## 实体定义

### CapabilityContract

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | UUID | 唯一主键 |
| `capability_key` | string | 能力命名（如 `crm.lead.create`），全局唯一 |
| `version` | string (SemVer) | 契约版本，遵循 `major.minor.patch` |
| `tenant_id` | string | 所属租户，空代表全局共享能力 |
| `provider_id` | string | 提供者标识（插件/Agent/服务） |
| `display_name` | string | 展示名称 |
| `description` | text | 描述 |
| `io_schema_id` | UUID | 指向 `IOSchemaDescriptor` |
| `security_scope` | string | 必需 Scope |
| `tool_grant_required` | bool | 是否要求 Tool Grant |
| `observability_config` | JSONB | 指标、日志、trace 维度 |
| `lifecycle_state` | enum(draft,published,deprecated) | 生命周期 |
| `effective_at` | timestamptz | 生效时间 |
| `deprecated_at` | timestamptz | 废弃时间，可为空 |
| `replacement_capability` | string | 推荐替代能力（可含版本） |
| `error_taxonomy_id` | UUID | 指向 `ErrorTaxonomy` |
| `transport_preferences` | JSONB | 每个协议的 prefer/only/fallback |
| `created_by` / `updated_by` | string | 审计信息 |
| `created_at` / `updated_at` | timestamptz | 时间戳 |

### IOSchemaDescriptor

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | UUID | 主键 |
| `contract_id` | UUID | 关联 `CapabilityContract` |
| `direction` | enum(input,output) | 输入或输出 |
| `format` | enum(json_schema,protobuf,avro) | 序列化格式 |
| `schema_uri` | string | 存储位置（Git、S3、内嵌） |
| `schema_hash` | string | 内容哈希（SHA256） |
| `validation_rules` | JSONB | 附加校验（必填、范围等） |

### CapabilityVersionPolicy

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | UUID | 主键 |
| `capability_key` | string | 能力命名，与契约多对一 |
| `default_strategy` | enum(latest_minor,fixed_major,custom) | 默认选版本策略 |
| `allowed_versions` | JSONB | 可用版本集合（含兼容标记） |
| `compatibility_matrix` | JSONB | 版本兼容矩阵（字段级差异、阻断原因） |
| `deprecation_policy` | JSONB | 废弃时间线、告警策略 |
| `audit_config` | JSONB | 审计与通知配置 |
| `updated_at` | timestamptz | 更新时间 |

### TransportProfile

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | UUID | 主键 |
| `contract_id` | UUID | 关联 `CapabilityContract` |
| `transport` | enum(http,grpc,mcp,agent) | 协议类型 |
| `mode` | enum(prefer,only,fallback) | 传输偏好 |
| `timeout_ms` | int | 超时时间 |
| `retry` | JSONB | 重试策略（次数、退避、幂等标记） |
| `streaming` | bool | 是否支持流式 |
| `qos` | JSONB | QoS 参数（并发、带宽、优先级） |
| `endpoint_selector` | JSONB | Router 提供的端点筛选条件 |
| `last_health_status` | JSONB | 最近健康检查结果（时间、状态、错误码） |

### ErrorTaxonomy

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | UUID | 主键 |
| `namespace` | string | 命名空间（如 `capability`） |
| `category` | string | 类别（`validation`、`transport`、`provider` 等） |
| `code` | string | 具体错误码（如 `input.missing_field`） |
| `severity` | enum(info,warning,error,fatal) | 严重级别 |
| `stage` | enum(validate,invoke,stream,observe) | 调用阶段 |
| `http_status` | int | HTTP 映射 |
| `grpc_status` | int | gRPC Status |
| `suggested_action` | text | 建议动作 |
| `telemetry_tags` | JSONB | 指标映射字段 |

### TransportRequest / TransportResponse / StreamChunk（运行时结构）

| 结构 | 关键字段 | 说明 |
| --- | --- | --- |
| `TransportRequest` | `request_id`、`trace_id`、`tenant_id`、`actor_id`、`capability_key`、`version`、`payload`、`deadline`、`retry_context`、`metadata` | 统一请求上下文 |
| `TransportResponse` | `request_id`、`status`、`output`、`error`、`metrics`、`observed_version` | 同步返回 |
| `StreamChunk` | `request_id`、`sequence`、`chunk_type`、`payload`、`timestamp`、`error` | 流式传输片段 |

## 关系与约束

- 一个 `CapabilityContract` **必须**关联两个 `IOSchemaDescriptor`（input/output），可扩展更多方向（如错误详情）。  
- `CapabilityContract` 与 `CapabilityVersionPolicy` 通过 `capability_key` 关联（1:N），策略记录兼容矩阵。  
- 每个 `CapabilityContract` 至少关联一个 `TransportProfile`，支持多协议配置。  
- `ErrorTaxonomy` 以 `namespace+category+code` 唯一约束，可被多个契约引用。  
- `TransportProfile.transport` 覆盖 http/grpc/mcp/agent，必须至少存在 http 与 grpc 两条记录（宪法要求）。  
- 所有表需携带 `tenant_id` 或 `provider_id` 以确保多租户隔离与审计。

## 状态机

```
draft ──(校验通过 / 发布审批)──▶ published ──(替代能力上线+缓冲期)──▶ deprecated
   ▲                                              │
   └───────────────(回滚/修改)────────────────────┘
```

- `draft → published`：需完成 Schema 校验、Scope/Grant 校验、兼容性评分 ≥90%。  
- `published → deprecated`：必须指定替代能力或版本，且生成 EventBus 告警事件。  
- `deprecated → published`：允许紧急回滚（同版本号），但需记录审计原因。

## 校验规则

- `capability_key` 使用小写点分命名，禁止空格/特殊字符。  
- `version` 必须合法 SemVer，禁止回退已发布版本（除紧急回滚案例）。  
- `transport_preferences` 中 `prefer` 和 `only` 互斥，同一协议不可重复配置。  
- `Stream` 能力需在 `IOSchemaDescriptor` 中标记流式输出 schema。  
- `retry` 配置如标记幂等 `idempotent=true` 才允许自动重试。  
- 所有 JSONB 字段写入前需校验结构与关键字段存在性（使用 JSON Schema）。  
- 契约发布前必须确保至少一个 `TransportProfile.streaming=true` 对应 FR-007 的流式标记（若能力声明支持流式）。  
- `ErrorTaxonomy` 需要覆盖常见错误类别（验证/调用/网络/提供者/系统），并确保与 HTTP/gRPC 状态映射一致。
