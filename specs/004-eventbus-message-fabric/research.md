# Research Notes: EventBus & Message Fabric

## Delivery Semantics & Dedupe
- **Decision**: 默认采用 At-Least-Once 投递，发布端写入 PostgreSQL 投递记录，订阅端显式 Ack/Nack，并结合 Redis 维护 5 分钟幂等窗口。
- **Rationale**: 满足规格中对可靠性和 99.9% 成功率的要求，同时易于实现高可用；Redis 去重窗口可快速过滤重复投递。
- **Alternatives considered**:
  - 恰好一次（二阶段提交 + 全局 offset）：实现复杂且回放需求会放大锁竞争，放弃。
  - 至多一次：无法满足核心业务一致性要求。

## 订阅传输协议
- **Decision**: 采用 gRPC 流式推送为默认订阅方式，保留 HTTP 长轮询作为兼容辅助（通过 Topic 配置开启）。
- **Rationale**: gRPC 在现有 CoreX 中已经使用，性能优于轮询，可复用统一拦截器与指标；轮询可兼容不支持 gRPC 的外部集成。
- **Alternatives considered**:
  - 仅提供轮询：无法满足低延迟目标。
  - WebSocket：需额外网关支持，部署复杂度高，留待后续扩展。

## 死信队列存储
- **Decision**: DLQ 消息持久化于 PostgreSQL（租户隔离表），同时在 Redis 写入轻量索引用于告警。
- **Rationale**: Postgres 满足审计、过滤、批量重放需求；与既有迁移体系对齐，易于备份。Redis 仅用于快速检测积压。
- **Alternatives considered**:
  - Redis list：易丢失数据且查询不便。
  - 对象存储：成本高且无法实时恢复。
  - Kafka DLQ Topic：目前未统一部署 Kafka，超出范围。

## 消息载荷格式
- **Decision**: 默认 JSON 序列化，并在 Topic 元数据中允许声明 `payload_format`（支持 Protobuf/Avro 扩展）。
- **Rationale**: JSON 对多语言生态友好，便于调试；通过元数据字段保留未来扩展空间。
- **Alternatives considered**:
  - 强制 Protobuf：外部消费者需同步生成代码，提升准入门槛。
  - 多格式并行：增加早期测试负担，暂缓。

## 重试与退避实现
- **Decision**: 基于 Redis Sorted Set 维护待重试队列，调度器按指数退避 + 抖动重投，同时写入 Postgres 投递日志。
- **Rationale**: Redis 支持高频调度与过期，满足 5000 msg/s；Postgres 保证审计完整性。
- **Alternatives considered**:
  - Cron/队列服务：新增基础设施，复杂度高。
  - 仅使用 Postgres 延迟任务：对数据库压力大，难以达到低延迟。
