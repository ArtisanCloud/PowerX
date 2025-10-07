# Phase 0 Research — Media Asset Admin Capabilities

**Source Spec**: [/private/var/www/html/ArtisanCloud/X/PowerX/Core/PowerX/specs/001-docs-media-storage/spec.md](spec.md)  
**Date**: 2025-10-07

## MediaAsset 唯一性与索引

- **Decision**: 保留 `PowerUUIDModel` 提供的 UUID 作为主键，并建立 `tenant_id + driver + object_key` 的唯一复合索引。
- **Rationale**: UUID 兼容现有模型和跨系统引用；复合索引可防止同一租户重复引用底层对象，满足审计与幂等需求。
- **Alternatives Considered**:
  - 仅依靠 UUID：无法阻止重复对象，运营端缺乏重复检测。
  - 使用自增 ID：不符合多服务之间共享资源的需求，破坏现有 UUID 约定。

## 分页与筛选性能

- **Decision**: 采用基于 MySQL InnoDB 的 offset/limit 分页，并对 `tenant_id + driver + status + updated_at` 建复合索引，标签筛选通过 JSON_CONTAINS 搭配前缀索引实现。
- **Rationale**: 后台规模（单租户 10 万级）仍可接受 offset/limit；组合索引覆盖常用过滤，`updated_at` 支撑倒序排序；标签采用 JSON 存储但可借助 `generated column` + 索引加速。
- **Alternatives Considered**:
  - Search 引擎（Elasticsearch）：对当前规模过度投入。
  - Keyset pagination：筛选组合复杂、对运营跳转翻页体验不佳。

## go-minio/S3 客户端实践

- **Decision**: 统一引入 `github.com/minio/minio-go/v7`，启用 TLS、请求重试（3 次指数退避），并使用临时会话 Token（STS）或短期 AK/SK。
- **Rationale**: minio-go 与 S3 兼容广泛，支持 presign；重试机制降低网络抖动；短期凭证降低泄露风险。
- **Alternatives Considered**:
  - AWS 官方 SDK：依赖过重，且在本地 MinIO 场景配置复杂。
  - 直接 HTTP 上传：丢失签名与分块能力。

## 标签字段与检索

- **Decision**: 标签存储在 `tags` JSON 列中，新增虚拟列 `tag_names` 存储排序后的字符串列表，并对其建立普通索引。
- **Rationale**: JSON 列保持灵活性；虚拟列索引支持多标签精准匹配；更新成本可控。
- **Alternatives Considered**:
  - 额外建交叉表：实现最严谨，但对当前标签需求（少量标签）略显复杂。
  - 纯 JSON 无索引：列表筛选性能难以保证。

## 软删后的物理清理

- **Decision**: 新增后台定时任务（现有 agent/cron 框架）每日扫描软删 7 天以上的资产，调用 MediaManager 执行物理删除并记录审计。
- **Rationale**: 保留窗口可供业务撤销；每日批处理负载低；审计记录满足追踪。
- **Alternatives Considered**:
  - 立即物理删除：不利于误删恢复。
  - 手工触发：运维成本高，易错漏。
