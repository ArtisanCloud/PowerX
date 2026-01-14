# DB Migrations — Vector Store & KG Assist Tables（make db-migrate）

## 背景 / 问题

当前 `make db-migrate` 仅执行 CoreX 的 GORM `AutoMigrate`（业务表），但 **不会**保证以下能力就绪：

- `pgvector` 扩展安装（`CREATE EXTENSION vector`）
- `knowledge_vectors_v{N}_{D}`（向量表族：按版本+维度分表）创建与索引创建
- KG / 图谱策略（`K_kg` 等）依赖的“协助表”（最小图谱存储：nodes/edges）

结果：即便入库链路已经把 embedding upsert 到 `VectorStore`，在本地/新环境中也会因为表/扩展不存在而阻塞或降级，且无法在 DB 工具中看到向量表。

## 目标

1. 运行 `make db-migrate` 后，**在同一 PostgreSQL 实例上**完成知识空间能力的基础 DB 准备：
   - `pgvector` 扩展存在
   - 向量表族（至少默认维度 `public.knowledge_vectors_v1_1536`）存在，且具备必要索引
   - 索引登记表（`public.knowledge_vector_indexes`）存在（用于 space→table 路由与治理）
   - KG 图谱“协助表”存在（最小 node/edge 表）
2. 迁移必须 **幂等**（可重复执行），且对无权限环境给出清晰错误信息（例如缺少 `CREATE EXTENSION` 权限）。
3. 对外部向量库（Milvus / Pinecone）场景：`make db-migrate` 不应强制创建 `knowledge_vectors_{D}`，但可创建 KG 协助表（取决于策略启用）。

## 非目标

- 不在本规格内要求实现 KG 构建算法、图谱抽取、或图检索链路；这里只定义“表与迁移准备”。
- 不要求一次性覆盖所有 RAG 模块的全部存储结构（例如 BM25 倒排、稀疏索引专用表等）；只先打通最小闭环与可观测性。

## 表设计（最小集）

### 1）向量表族：`public.knowledge_vectors_v{N}_{D}`（pgvector）

用途：为 `backend/pkg/corex/db/persistence/vectorstore/pgvector` 提供 dense 落表，支持按 space 维度 upsert/query，并允许未来切换 embedding 模型维度。

建议 DDL（模板；与代码一致，细节以最终 migration 为准）：

```sql
CREATE EXTENSION IF NOT EXISTS vector;

-- 默认版本+维度（例如 v1 + 1536）
CREATE TABLE IF NOT EXISTS public.knowledge_vectors_v1_1536 (
  space_uuid uuid NOT NULL,
  chunk_uuid uuid NOT NULL,
  embedding vector(1536) NOT NULL,
  metadata jsonb,
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (space_uuid, chunk_uuid)
);

CREATE INDEX IF NOT EXISTS knowledge_vectors_v1_1536_space_idx
  ON public.knowledge_vectors_v1_1536 (space_uuid);

-- 可选：近邻索引（IVFFLAT / HNSW，按 pgvector 版本与运营策略选择）
CREATE INDEX IF NOT EXISTS knowledge_vectors_v1_1536_embedding_idx
  ON public.knowledge_vectors_v1_1536 USING ivfflat (embedding vector_l2_ops) WITH (lists = 100);
```

### 1.1）索引登记表：`public.knowledge_vector_indexes`

用途：记录每个 space 当前激活的向量索引（维度/表名/provider/model），并用于垃圾回收与回滚治理。

```sql
CREATE TABLE IF NOT EXISTS public.knowledge_vector_indexes (
  id bigserial PRIMARY KEY,
  space_uuid uuid NOT NULL,
  index_key varchar(128) NOT NULL,
  table_name varchar(128) NOT NULL,
  dimensions int NOT NULL,
  embedding_provider varchar(64) NOT NULL,
  embedding_model varchar(128) NOT NULL,
  embedding_profile_ref varchar(128),
  status varchar(32) NOT NULL DEFAULT 'active',
  last_used_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS knowledge_vector_indexes_space_key_uniq
  ON public.knowledge_vector_indexes(space_uuid, index_key);

CREATE INDEX IF NOT EXISTS knowledge_vector_indexes_space_status_idx
  ON public.knowledge_vector_indexes(space_uuid, status);
```

> 迁移策略建议：
> - `make db-migrate` 只保证：`vector` 扩展 + `knowledge_vector_indexes` + 默认版本+维度表（例如 `v1_1536`）存在。
> - 其它维度表（例如 1024/768）由“管理员在 AI Settings probe 成功后，把该 embedding profile 绑定到 space 并激活”时按需创建（避免无谓建表）。
> - AI Settings 的“连接测试/试跑”阶段 **不做建表**，只负责拿到维度并写回 profile，用于后续 space 级激活的强校验。

---

## RAG 策略模块的存储依赖（就绪矩阵）

SSOT（后端与 Web Admin 的依赖校验来源）：
- `backend/config/knowledge/scene_strategy_catalog.yaml`（`modules.*.requires`、`strategy_bundles.*.prerequisites`、`scenes.*.prerequisites`）

本节目标：把 `requires/prerequisites` 映射为 **是否需要 DB 辅助表**，以及是否应纳入 `make db-migrate`。

### Index Prerequisites → 存储实现建议

| prerequisite | 含义 | 建议存储 | 是否建议纳入 `db-migrate` |
| --- | --- | --- | --- |
| `index.dense` | 向量索引 | 外部向量库（Milvus/Pinecone）或 Postgres(pgvector) | ✅（当 driver=pgvector 时创建默认 `knowledge_vectors_v1_1536`；其余维度按需创建） |
| `index.sparse` | 稀疏索引 / BM25 / FTS | 外部搜索（ES/OS）或 Postgres FTS | ✅（若选择 Postgres FTS 方案：创建 `knowledge_chunks` + FTS 索引） |
| `index.hier` | 层次化索引（doc/section/chunk） | Postgres 结构表（chunk store + relations）或对象存储+计算 | ✅（若启用 hier：建议创建 `knowledge_chunks`/`knowledge_chunk_links`） |
| `index.kg` | 知识图谱索引 | Postgres 图谱协助表（nodes/edges）或图数据库 | ✅（至少创建 `knowledge_kg_nodes/edges`） |
| `index.time_fields` | 时间/版本字段 | Postgres（列/索引）或仅 metadata | △（先落 `metadata`，必要时补索引列） |
| `index.structured_fields` | 表格/字段过滤 | Postgres jsonb + GIN 或外部（列存/OLAP） | △（可先用 jsonb+GIN；如无需求可不建） |

### Asset / Runtime prerequisites

| prerequisite | 说明 | 存储建议 |
| --- | --- | --- |
| `asset.section_summaries` | section/doc 摘要产物 | MinIO/S3（ArtifactBundle）即可；不强制 DB 表 |
| `asset.augmented_fields` | 离线增强字段（关键词/Q&A/实体标签等） | MinIO/S3（ArtifactBundle）+（可选）写回 chunk metadata |
| `runtime.*` | rerank/llm/evidence_checker 等运行时能力 | 不是 DB 迁移职责（依赖配置/服务可用性） |

> 结论：**KG 必须有 DB（或图数据库）落点**；dense/sparse/hier 是否落 Postgres 取决于你们的索引选型，但如果当前默认就想“开箱可用”，至少应提供 Postgres 方案并纳入 `db-migrate`。

---

## RAG 模块 → 存储落点矩阵（B 方案：Postgres-backed 默认实现）

> 说明：你提到的“十几个 RAG 策略（A–O）”不建议做成“每个模块各建一套专属表”。更合理的做法是：
> - 把“需要建表的部分”抽象为 **索引/资产底座**（dense / sparse / hier / kg / metadata）
> - 各模块复用同一套底座表 + 对象存储产物 + 运行时能力（LLM/rerank/evidence_checker）
>
> 下表把每个模块映射到“依赖哪些底座能力”，并给出在 B 方案下应落到哪些表（或对象存储），以及 `make db-migrate` 是否应准备对应表。

约定（B 方案默认）：
- `index.dense`（pgvector）→ `public.knowledge_vectors_v{N}_{D}`（仅当 `driver=pgvector`；N 为 schema 版本，D 与 space 绑定的 embedding profile 一致）
- `index.sparse`（Postgres FTS）→ `public.knowledge_chunks`（FTS/GIN）
- `index.hier`（邻接/层次）→ `public.knowledge_chunks(kind=doc_summary/section_summary/...)` + `public.knowledge_chunk_links`
- `index.kg`（Postgres KG）→ `public.knowledge_kg_nodes` / `public.knowledge_kg_edges`
- `index.time_fields` / `index.structured_fields` → 优先落 `public.knowledge_chunks.metadata(jsonb)`，必要时再演进专列/专表
- `asset.*` → 默认由 `ArtifactBundle`（MinIO/S3）承载；是否落 DB 取决于治理需求
- `runtime.*` → 运行时能力，不属于 DB 迁移职责（但需要 readiness check）

### 模块映射表（SSOT：scene_strategy_catalog.yaml）

| 模块 | `requires`（来自 SSOT） | 主要存储落点（B 方案） | `db-migrate` 是否需要建表 |
| --- | --- | --- | --- |
| A_simple（Simple RAG） | `index.dense` | `knowledge_vectors_v{N}_{D}`（或外部向量库） | ✅（仅 pgvector 时建默认维度表；其余按需） |
| A1_routing（Query Routing） | — | 无（路由规则可在配置/策略表中管理，非必需） | ❌ |
| A2_time_aware（Time-aware） | `index.time_fields` | `knowledge_chunks.metadata`（如 `effective_from/to`、`doc_version`）；必要时加索引列 | △（默认不新增表；如需专列/索引再加迁移） |
| B_semantic_chunking（Semantic Chunking） | `asset.section_summaries` | `ArtifactBundle` 产物；（可选）摘要写入 `knowledge_chunks(kind=section_summary)` | ❌（默认不新增表；若采用 `knowledge_chunks` 则已覆盖） |
| C_context_enriched（Context Enriched） | `index.hier` | `knowledge_chunks` + `knowledge_chunk_links`（邻接扩展） | ✅（启用 hier 时需建 `knowledge_chunk_links`；`knowledge_chunks` 作为底座建议总建） |
| D_doc_augmentation（Doc Augmentation） | `asset.augmented_fields` | `ArtifactBundle` 增强产物；（推荐）关键增强字段回写 `knowledge_chunks.metadata` 供过滤/召回 | ❌（默认不新增表；如做“增强资产版本治理”可另建资产表） |
| E_query_transform（Query Transformation） | — | 无（运行时策略） | ❌ |
| F_rerank（Reranker） | `runtime.rerank` | 无（运行时模型/服务） | ❌ |
| G_rse（RSE） | `asset.domain_lexicon` | `ArtifactBundle`（词表/实体库版本）；（可选）词表摘要写入 DB | ❌（默认不新增表；如做词表管理可建 `domain_lexicon_versions` 等） |
| H_fusion（Fusion） | `index.dense`, `index.sparse` | `knowledge_vectors_v{N}_{D}` + `knowledge_chunks`（FTS/GIN） | ✅（pgvector 时建默认维度表；FTS 方案建 chunks） |
| I_hyde（HyDE） | `runtime.llm`, `index.dense` | 仅复用 `knowledge_vectors_v{N}_{D}`（HyDE 生成的“假设文档”用于 embedding） | ✅（仅 pgvector 时建默认维度表；其余按需） |
| J_hier（Hierarchical Indices） | `index.hier`, `asset.section_summaries` | `knowledge_chunks(kind=doc_summary/section_summary/chunk)` + `knowledge_chunk_links`；摘要也可在 S3 | ✅（同 C；summary 产物不强制 DB 迁移） |
| K_kg（Knowledge Graph） | `index.kg` | `knowledge_kg_nodes` / `knowledge_kg_edges`（+ 可选 provenance 映射） | ✅（必须） |
| L_feedback（Feedback Loop） | `runtime.feedback` | 业务表已存在：`knowledge_feedback_cases` + `knowledge_ingestion_jobs` + `knowledge_artifact_bundles` 等 | ✅（已在 CoreX AutoMigrate 覆盖；无需新增表） |
| M_adaptive（Adaptive RAG） | `runtime.policy_router` | 无（运行时策略） | ❌ |
| N_self_rag（Self RAG） | `runtime.llm`, `runtime.evidence_checker` | 无（运行时回路）；证据来源复用 sparse/hier/kg 召回结果 | ❌ |
| O_crag（CRAG） | `runtime.evidence_checker`, `index.sparse` | 证据纠错依赖 sparse：`knowledge_chunks`（FTS/GIN） | ✅（FTS 方案建 `knowledge_chunks`） |

> 关键点回答你担心的地方：
> - 不是“其它模块只要向量表就行”。它们往往复用 **同一套底座表**：`knowledge_chunks`（sparse/structured）、`knowledge_chunk_links`（hier/context）、`knowledge_kg_*`（kg）、`knowledge_vectors_v{N}_{D}`（dense）。
> - 真正需要“新增专属表”的，通常是你想做资产治理/版本化时（例如 domain lexicon、augmentation artifacts 的 DB 管理），这属于后续增强而非跑通 RAG 的必要条件。

## 推荐的“统一 Chunk Store”（支撑 sparse + hier + 结构化过滤）

为避免 sparse/hier/结构化过滤各自建一套表，建议在 Postgres 内引入一个最小的 chunk store（可选，但强烈推荐作为企业默认落地）：

### 3）Chunk Store：`public.knowledge_chunks`

用途：
- 为 `index.sparse`（FTS/BM25）提供可检索文本
- 为 `index.hier`（doc/section/chunk）提供结构化关系与邻接扩展
- 为 `index.structured_fields` 提供 jsonb 过滤与索引

建议 DDL（最小可用）：

```sql
CREATE TABLE IF NOT EXISTS public.knowledge_chunks (
  space_uuid uuid NOT NULL,
  chunk_uuid uuid NOT NULL,
  kind text NOT NULL DEFAULT 'chunk', -- doc_summary/section_summary/chunk/...
  content text NOT NULL,
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb, -- provenance/anchors/fields/time...
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (space_uuid, chunk_uuid)
);

-- 稀疏检索（Postgres FTS）
CREATE INDEX IF NOT EXISTS knowledge_chunks_fts_idx
  ON public.knowledge_chunks USING gin (to_tsvector('simple', content));

-- 结构化过滤（jsonb contains）
CREATE INDEX IF NOT EXISTS knowledge_chunks_meta_idx
  ON public.knowledge_chunks USING gin (metadata jsonb_path_ops);

CREATE INDEX IF NOT EXISTS knowledge_chunks_kind_idx
  ON public.knowledge_chunks (space_uuid, kind);
```

### 4）层次/邻接关系：`public.knowledge_chunk_links`（可选）

用途：Context Enriched / Hier 的邻居扩展、parent-child 下钻、回滚时的成组删除。

```sql
CREATE TABLE IF NOT EXISTS public.knowledge_chunk_links (
  space_uuid uuid NOT NULL,
  src_chunk_uuid uuid NOT NULL,
  dst_chunk_uuid uuid NOT NULL,
  rel_type text NOT NULL, -- parent/next/prev/derived_from/...
  props jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (space_uuid, src_chunk_uuid, dst_chunk_uuid, rel_type)
);

CREATE INDEX IF NOT EXISTS knowledge_chunk_links_space_src_idx
  ON public.knowledge_chunk_links (space_uuid, src_chunk_uuid);
```

> 注意：引入 `knowledge_chunks` 不等于要废弃 MinIO/S3 的 ArtifactBundle；ArtifactBundle 仍然承担 lineage、checksum、离线产物版本化的职责。Chunk Store 负责在线检索加速与结构化能力。

### 2）KG 协助表：`public.knowledge_kg_nodes` / `public.knowledge_kg_edges`

用途：为 `K_kg`（知识图谱）等策略提供最小的结构化存储面，使入库/再处理阶段能把抽取结果落地并可回滚/清理。

建议 DDL（最小可用）：

```sql
CREATE TABLE IF NOT EXISTS public.knowledge_kg_nodes (
  space_uuid uuid NOT NULL,
  node_id text NOT NULL,
  node_type text NOT NULL DEFAULT 'entity',
  props jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (space_uuid, node_id)
);

CREATE TABLE IF NOT EXISTS public.knowledge_kg_edges (
  space_uuid uuid NOT NULL,
  edge_id text NOT NULL,
  src_node_id text NOT NULL,
  dst_node_id text NOT NULL,
  predicate text NOT NULL,
  props jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (space_uuid, edge_id)
);

CREATE INDEX IF NOT EXISTS knowledge_kg_edges_space_idx
  ON public.knowledge_kg_edges (space_uuid);
CREATE INDEX IF NOT EXISTS knowledge_kg_edges_src_idx
  ON public.knowledge_kg_edges (space_uuid, src_node_id);
CREATE INDEX IF NOT EXISTS knowledge_kg_edges_dst_idx
  ON public.knowledge_kg_edges (space_uuid, dst_node_id);
```

> 注：这里采用 `text` 作为 node/edge id，便于对接多来源（SQL AST、配置对象、实体抽取）并保持稳定性；后续可演进为 `uuid` 或拆分实体表。

## 迁移执行策略（与 `make db-migrate` 对齐）

### 输入来源

- `make db-migrate` → `backend/cmd/database migrate` → `database.MigrateCoreModels(db)`
- 需要扩展为：在 **同一次 db-migrate** 中执行 Knowledge Space 的存储准备。

### 配置开关（默认 B 方案）

为避免“默默创建无用表”，本实现使用显式配置决定是否创建 chunk store：

```yaml
knowledge_space:
  index_backends:
    sparse: postgres_fts        # or external
    hier: postgres_links        # or external
    structured_fields: postgres_jsonb # or external
    kg: postgres                # or external
```

`make db-migrate` 的行为：
- 当 `sparse=postgres_fts` 或 `structured_fields=postgres_jsonb` 时创建 `knowledge_chunks`
- 当 `hier=postgres_links` 时创建 `knowledge_chunk_links`

### 行为约定

1. 若 `knowledge_space.vector_store.driver == "pgvector"`：
   - 使用 `knowledge_space.vector_store.pgvector.dsn`（若为空则复用 `database.dsn`）连接目标库
   - 执行 pgvector migration：`CREATE EXTENSION vector` + `knowledge_vectors_v1_1536`（默认）+ 索引 + `knowledge_vector_indexes`
2. 若 driver ≠ `pgvector`：
   - **不创建** `knowledge_vectors_{D}`（避免误导与浪费）
3. KG 协助表创建：
   - 默认创建（成本低、且不依赖扩展）
   - 或者由 feature flag / scene strategy 触发（例如启用 `K_kg` bundle 才创建）

4. Sparse/Hier/Structured（若采用 Postgres 方案）：
   - `index.sparse` / `index.hier` / `index.structured_fields` 若在你的 `IndexProfile` 中被启用，则 `make db-migrate` 应创建 `knowledge_chunks`（以及可选的 `knowledge_chunk_links`）与相关索引。

## 验收标准

- 执行 `make db-migrate` 后，以下查询应返回存在：

```sql
select to_regclass('public.knowledge_vectors_v1_1536') as knowledge_vectors_v1_1536;
select to_regclass('public.knowledge_vector_indexes') as knowledge_vector_indexes;
select to_regclass('public.knowledge_chunks') as knowledge_chunks;
select to_regclass('public.knowledge_chunk_links') as knowledge_chunk_links;
select to_regclass('public.knowledge_kg_nodes') as knowledge_kg_nodes;
select to_regclass('public.knowledge_kg_edges') as knowledge_kg_edges;
```

- 再执行一次 `make db-migrate` 不应报错（幂等）。
