# Phase 0 Research — Tool Grants & Security Policy

## 授权评估缓存策略

- **Decision**: 采用 Redis Cluster 作为跨实例共享缓存，结合本地 LRU（10s）双层缓存；缓存键包含 `tenant_id`、`subject_id`、`grant_version`。
- **Rationale**: Redis 支持毫秒级 TTL 与 Pub/Sub，可在 Grant 失效时推送失效消息；双层缓存兼顾性能与一致性，满足 50ms 平均延迟目标。
- **Alternatives considered**:
  - 仅使用应用内缓存：跨实例不一致，Grant 失效传播慢。
  - 使用数据库查询：性能不足，无法满足 P99 < 200ms。

## Challenge 审批编排

- **Decision**: Challenge 审批事件投递到现有企业 SOAR 队列（Kafka 主题 `secops.challenge`），由安全运营团队工单系统承接，超时由授权服务主动拒绝。
- **Rationale**: 复用现有安全运营流程，便于审计追踪；Kafka 可提供重放能力配合 3 年留存；超时策略与 SLA 目标一致。
- **Alternatives considered**:
  - 新建专用审批微服务：增加维护成本，与企业现有流程重复。
  - 手工邮件通知：缺乏可追踪性，不符合审计要求。

## 审计留存与冷存储

- **Decision**: 授权与评估事件实时写入 ClickHouse（在线检索），每日批量归档到对象存储（S3 兼容）并保留 3 年；归档索引写入 Metadata 表。
- **Rationale**: ClickHouse 擅长大规模日志分析且支持多租户分区；对象存储具备低成本长留存；Metadata 方便稽核导出。
- **Alternatives considered**:
  - 仅写入 Elasticsearch：长期留存成本高，冷热分层复杂。
  - 直接写入数据湖：查询延迟高，影响审计及时性。

## gRPC/HTTP 安全拦截器

- **Decision**: gRPC 服务使用通用拦截器链（认证、租户注入、速率限制、审计），HTTP Handler 复用安全中间件并扩展 RBAC 校验；两者共享 `authorization.Service`。
- **Rationale**: 统一拦截器降低重复实现；可直接复用 Constitution 要求的 logging/tracing；便于满足默认拒绝策略。
- **Alternatives considered**:
  - 独立实现两套拦截器：增加维护负担，易出现策略不一致。
  - 将授权前移至 API Gateway：缺乏领域上下文信息，难以执行细粒度条件。
