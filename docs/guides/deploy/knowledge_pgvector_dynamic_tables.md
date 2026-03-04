# 部署：pgvector + 动态向量表（按维度分表）

本文描述 Knowledge Space 的 Dense 向量索引在 Postgres(pgvector) 下的部署与运维要点，重点是“按维度分表 + 空间激活建表”的行为约束。

## 1) 关键结论（你只需要记住这几条）

- Dense 向量表是 **全局共享表**（通过 `space_uuid` 隔离），不会按 tenant/space 单独建表。
- Dense 向量表按维度分表：`knowledge_vectors_v1_<D>`（例如 `knowledge_vectors_v1_1536`）。
- **AI Settings 的 embedding 测试连接会建表**：完成 probe 后会创建 `knowledge_vectors_v1_<D>`（若不存在），并写回 `ai_model_profiles.cap_cache`（`probed_at`/`dimensions`）。
- **建表发生在 Space 激活向量索引（Dense）时**：`probe → CREATE TABLE IF NOT EXISTS → 写 registry → 绑定到 space`。

## 2) 数据库依赖

- Postgres 需要安装 pgvector 扩展（`extname=vector`）。
- DB 用户需要具备 `CREATE EXTENSION` 权限（如果你要让系统自动创建扩展）。

## 3) 配置（config.yaml）

### 3.1 默认向量表（迁移阶段创建）

`knowledge_space.vector_store.pgvector.table` 建议设置为默认表（通常 1536 维）：

```yaml
knowledge_space:
  vector_store:
    driver: pgvector
    pgvector:
      schema: public
      table: knowledge_vectors_v1_1536
      dimensions: 1536
```

说明：
- `make db-migrate` / `make db-refresh` 会创建该默认表（以及 extension/index）。
- 其它维度表不会在迁移阶段批量创建，避免“错误配置导致垃圾表爆炸”。

### 3.2 动态建表（Space 激活时）

当某个 space 激活 Dense 向量索引时，后端会按 probe 得到的维度创建：
- `public.knowledge_vectors_v1_<D>`（幂等：`IF NOT EXISTS`）
- 并写入 `powerx.knowledge_vector_indexes` 用于路由与治理

## 4) 验证方式

### 4.1 迁移后检查默认表

在 Postgres 执行：

```sql
select to_regclass('public.knowledge_vectors_v1_1536');
select 1 from pg_extension where extname='vector';
```

### 4.2 Space 激活后检查

```sql
-- 查看 space 是否已绑定
select uuid, embedding_profile_key, active_vector_index_key
from powerx.knowledge_spaces
where uuid = '<space_uuid>';

-- 查看 registry
select space_uuid, index_key, table_name, dimensions, status, last_used_at
from powerx.knowledge_vector_indexes
where space_uuid = '<space_uuid>'
order by created_at desc;
```

## 5) 常见问题

### 5.1 入库显示 degraded / 无向量

常见原因：
- space 没激活 Dense 索引（`embedding_profile_key`/`active_vector_index_key` 为空）
- embedding profile 尚未完成 probe（`cap_cache.probed_at`/`dimensions` 为空）
- embedding provider 没有配置可用凭据（AI Settings 未配置或配额耗尽）

### 5.2 想换 embedding provider/model

当前策略是 **space 级锁定 embedding profile**：
- 换模型建议走“新激活（生成新 index_key）→ 重新入库/重建向量”路径
- 同一 space 混用不同 embedding 模型会导致向量空间不一致，检索质量不可控
