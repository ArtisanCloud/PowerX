# Phase 0 Research — Media Asset Admin Capabilities (PowerX / Postgres / GORM)

**Source Spec**: /specs/001-docs-media-storage/spec.md  
**Date**: 2025-10-08

> 本研究文档修正了数据库相关表述，统一以 **GORM 抽象层 + Postgres 默认环境** 为前提；去除 MySQL/InnoDB/JSON_CONTAINS 等措辞，改为 Postgres/JSONB/GIN 的实践。

---

## 1. 唯一性与核心索引

**Decision**  

- 主键：沿用 `PowerUUIDModel`（UUID）。  
- 业务唯一：对 `tenant_id, driver, object_key` 建立**唯一约束**，保证同一租户下同一驱动的对象键不重复。

**Rationale**  

- UUID 统一跨服务引用；复合唯一避免重复引用底层对象（满足审计、幂等）。

**DDL（Postgres 示例）**  

```sql
ALTER TABLE media_assets
  ADD CONSTRAINT uq_media_tenant_driver_key UNIQUE (tenant_id, driver, object_key);
CREATE INDEX idx_media_tenant_driver_status_updated
  ON media_assets (tenant_id, driver, status, updated_at DESC, id DESC);
```

**Alternatives Considered**  

- 仅 UUID 唯一：无法拦截重复对象键。  
- 额外使用 HASH 对象键：复杂度提升，收益有限。

---

## 2. 分页与筛选性能（Admin 规模 ≤ 10^5/tenant）

**Decision**  

- 默认使用 **Offset/Limit**（后台检索友好、可跳页）。  
- 深翻页/Feed 流场景提供 **Keyset/Seek** 作为可选优化：`ORDER BY updated_at DESC, id DESC`；游标采用 `(last_updated_at, last_id)`。  
- 组合索引：`(tenant_id, driver, status, updated_at DESC, id DESC)` 覆盖常用过滤；若 `driver` 选择较少，可根据实际把 `driver` 放末尾或使用多索引策略。

**Rationale**  

- 10 万量级/租户下，Offset 在合理索引下可接受；Keyset 面向“更多/上一页”体验且稳定。

**Keyset 查询示例（Postgres）**  

```sql
SELECT *
FROM media_assets
WHERE tenant_id = $1
  AND driver = $2
  AND status = $3
  AND (updated_at, id) < ($4, $5)  -- cursor tuple
ORDER BY updated_at DESC, id DESC
LIMIT $6;
```

**Alternatives Considered**  

- 直接接入搜索引擎（ES/PG-Search）：对当前规模过度投入。

---

## 3. 标签字段与检索（JSONB + GIN）

**Decision**  

- `tags` 使用 **JSONB** 数组（如 `["banner","autumn"]`），建立 **GIN 索引**。  
- **“包含全部标签（AND）”** 检索用 `@>` 操作符；**“包含任一标签（OR）”** 可退化为多次查询或构造 `OR` 条件。

**DDL（Postgres）**  

```sql
-- 建议：tags 为空时存储为 '[]'，不要为 NULL
ALTER TABLE media_assets
  ALTER COLUMN tags SET DEFAULT '[]'::jsonb,
  ALTER COLUMN tags SET NOT NULL;

-- GIN 索引（默认 opclass 已可用；如需更小索引可用 jsonb_path_ops）
CREATE INDEX idx_media_tags_gin ON media_assets USING GIN (tags);
```

**查询示例**  

```sql
-- 包含全部标签（AND）
SELECT * FROM media_assets WHERE tenant_id=$1 AND tags @> '["banner","autumn"]';

-- 包含任一标签（OR） —— 方案A：OR 组合
SELECT * FROM media_assets WHERE tenant_id=$1 AND (tags @> '["banner"]' OR tags @> '["autumn"]');
```

**Rationale**  

- JSONB + GIN 对“包含”类查询性能好、灵活度高；避免维护交叉表复杂度。

**Alternatives Considered**  

- TEXT[] + GIN：实现也可行，但与未来 `meta` JSONB 生态割裂。  
- 交叉表（多对多）：可获得更严谨的统计与约束，但当前标签轻量，不急于引入。

---

## 4. go-minio / S3 客户端实践

**Decision**  

- 统一 `github.com/minio/minio-go/v7`；启用 TLS；设置连接/读写超时与**重试（指数退避，建议 3 次）**；  
- 使用 **短期凭证（STS）/短期 AK/SK**；Presign 默认 12h，可配置；  
- 大对象启用分片上传；可选开启 **SSE-S3/SSE-C**；Bucket 在启动时校验/自动创建（可配置开关）。

**Rationale**  

- 与 S3/OSS/OBS 兼容强；Presign 与分片能力成熟；易本地（MinIO）复现。

**建议实践**  

- Presign: GET/PUT 仅允许受管路径与 Content-Type 白名单；  
- PutObject: 设置合理 `PartSize`（如 16–64MB）与 `ContentType`；  
- 失败重试：仅对幂等操作（GET/HEAD）与安全的 PUT 重试；避免对删除等非幂等操作盲目重试。

---

## 5. 软删除与物理清理（Retention & Idempotency）

**Decision**  

- **软删**：API 仅置 `deleted_at`，并记录审计（操作者、时间、资产摘要）。  
- **清理任务**：由现有**后台任务框架/定时器**每日扫描 `deleted_at <= now() - interval '7 days'` 的资产，调用 MediaManager 执行对象删除；失败写入告警并重试。  
- **幂等性**：驱动删除应视为幂等（对象不存在不报错）。

**示例（Postgres 扫描）**  

```sql
SELECT id, driver, bucket, object_key
FROM media_assets
WHERE deleted_at IS NOT NULL
  AND deleted_at <= now() - interval '7 days'
LIMIT 1000; -- 批处理窗口
```

**Rationale**  

- 保留 7 天回滚窗口；批处理负载低；审计可追溯。

**Alternatives Considered**  

- 立即物理删除：误删恢复困难。  
- 手动清理：运维成本高。

---

## 6. 多租户与安全（RBAC / 审计 / Least-Privilege）

**Decision**  

- 所有读取与修改操作必须带 **tenant_id** 上下文过滤；  
- 预签名链接仅对**已授权的管理员**开放；默认过期 12h；限制 HTTP Method 与 MIME；  
- 所有关键操作写入**审计日志**（操作人、租户、路径、参数摘要、结果）。

**Rationale**  

- 符合 PowerX 的多租户安全与最小权限原则；可追溯。

---

## 7. 兼容性与可迁移性

**Decision**  

- **数据库无关**：通过 **GORM** 抽象适配；默认 Postgres；如切换 MySQL，仅需迁移 JSONB → JSON 与索引策略（新项目不建议）。  
- **配置无关**：驱动配置集中在 `config/storage.go`；MediaManager 通过依赖注入使用。

**Rationale**  

- 降低环境绑定，便于插件化与不同部署形态。

---

## 8. 开放问题（进入 Phase 1 前需确认）

1) **标签语义**：是否必须支持“任意标签（OR）+ 全部标签（AND）+ 排除标签（NOT）”的组合查询？若是，需在列表 API 统一表达查询 DSL。  
2) **对象生命周期策略**：是否需要 per-tenant/per-folder 的生命周期（如 N 天后转低频、N+M 天后自动清理）？如需，可在 `meta`/策略表扩展。  
3) **外链直传回填**：是否允许通过 `file_url` 创建资产但不落库对象（仅登记外链）？如需，需在 Service 层增加可达性校验策略。  
4) **重复检测**：是否需要基于 `sha256` 的去重（同一租户重复文件覆盖或提示）？若需要，需为 `sha256` 建索引。

---

## 9. 小结

- 采用 **UUID + (tenant_id, driver, object_key) 唯一约束**，保证幂等。  
- 使用 **Postgres JSONB + GIN** 实现灵活与高性能的标签检索；分页采用 Offset，必要时提供 Keyset 优化。  
- S3 客户端统一 **minio-go/v7**，保证 TLS、超时、重试、分片、Presign；删除走定时清理、幂等与审计。  
- 全链路多租户安全（RBAC）与可观测性满足 PowerX 宪章要求。
