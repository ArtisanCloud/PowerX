# 存储与索引设计（Storage_and_Indexing）

> 文档状态：Draft v0.4  
> 维护者：PowerX CoreX 团队  
> 上次更新：2025-10-13

---

## 1. 设计目标

- **低耦合可替换**：Embedding 引擎与向量数据库可插拔（OpenAI/BGE… × Qdrant/pgvector/Milvus）。
- **强一致与可追踪**：所有摄入/解析/索引动作可审计、可回放、可重建。
- **高吞吐可观测**：异步流水线、批处理友好，具备任务/链路级指标。
- **多租户隔离**：从对象存储路径、数据库 schema 到向量集合名，均含租户维度。

---

## 2. 分层存储结构

PowerX 知识库存储分为三层（逻辑层次，不强绑定物理实现）：

1) **元数据层（PostgreSQL）**  
   - 管理 `KnowledgeSpace/Document/Version/Chunk/Embedding` 元信息与状态；  
   - 可选 `pgvector` 仅用于小规模或本地化场景（否则向量落外部库）；  
   - 统一承载审计与多租户隔离。

2) **对象存储层（CoreX Media Manager）**  
   - 原始文件与解析产物（结构化 JSON、增量快照、切分结果）；  
   - 统一路径：`knowledge/{tenant}/{document}/{version}/…`；  
   - 支持 Local / S3 兼容。

3) **检索与缓存层（可选）**  
   - 关键词倒排：Postgres `tsvector` 或 OpenSearch；  
   - 高频结果缓存：Redis（Top-N chunk id 列表、短期上下文块）；  
   - L2 缓存：热点文档解析结果 JSON 化并缓存在对象存储。  

> 注：你上传的版本已描述了三层结构，本节在此基础上固化命名与职责，便于 SDK 与运维统一实现。:contentReference[oaicite:1]{index=1}

---

## 3. 数据流（Ingest → Index → Serve）

### 3.1 流程总览

```mermaid
flowchart LR
    A[Ingest: Upload/Sync]
    B[Parse: Parser & Chunker]
    C[Embed: Embedding Engine]
    D[Persist: Embedding Registry]
    E[Upsert: Vector Store]
    F[Keyword Index]
    G[Publish: doc.indexed]
    H[Cache Warm: Redis & Object Store]

    A --> B
    B --> C
    C --> D
    C --> E
    D --> F
    E --> F
    F --> G
    G --> H

```

**阶段说明：**

- **Ingest**：通过 API/SDK 创建 `KnowledgeDocument` + `DocumentVersion`，写入对象存储；
- **Parse**：`DocumentParser` + `Chunker` 切分为 `DocumentChunk`，抽取 keywords/entities；
- **Embed**：按 Space/Source 绑定的模型生成向量；
- **Persist/Upsert**：

  - 在 PG `embeddings` 表登记元数据/状态（Embedding Registry）；
  - 经 `VectorStoreAdapter` 写入外部向量库（Qdrant/Milvus/pgvector…）；
- **Keyword Index**：写入 tsvector 或 OpenSearch；
- **Publish**：发布 `knowledge.document.indexed` 供 Agent/Flow/Cache 订阅预热；
- **Serve**：检索服务对外提供 Hybrid 检索与重排。

> 本节细化了你已写的数据流 1-5 步的职责边界，并加入了 *Embedding Registry ↔ VectorStore* 的双轨持久化过程。

---

## 4. 任务编排与容错

### 4.1 任务切分

- 标准步骤：`Parse` → `Chunk` → `Embed` → `Persist/Upsert`；
- 每步独立幂等（**idempotency_key** = `${tenant}:${document_id}:${version}:${step}:${checksum}`）；
- 支持**单步重试**与**阶段回退**（例如重新 Embed 不会重复 Parse）。

### 4.2 重试与死信

- 默认 **3 次指数退避**（2^n × baseDelay，n∈[0,2]）；
- 超阈进入 **DLQ**，触发 `knowledge.indexing.failed` 告警事件（PagerDuty/Slack Hook）；
- 运维可在 Admin「任务面板」点选**重放**或**跳过**。

### 4.3 并发与节流

- 全局 `max_concurrency`，按租户与空间支持 token-bucket；
- 按模型供应方（如 OpenAI）配置速率限制，避免 429；
- 大文件/大批量采用 **batch embed** 接口（N×文本 → N×向量）。

---

## 5. 片段切分策略（Chunking）

- **默认**：基于 token 的滑动窗口（`window=512`, `overlap=64`）；
- **结构化**：表格/FAQ 使用规则切分，保证字段语义完整；
- **可配置**：在 `KnowledgeSpace.settings.chunker` 绑定 `default/pdf/web/faq` 等策略；
- **插件扩展**：实现 `Chunker` 接口注册为 `chunker.{name}` 并在 Space/Source 级别启用。

---

## 6. Embedding 管理（Embedding Registry）

### 6.1 为什么需要 Registry

- 记录**向量来自哪个 chunk、用了哪个模型、维度是多少、质量评分如何**；
- 支持**多模型并存**与离线 A/B 比较；
- 便于**迁移/重建**（向量库更换时，仅按 Registry 回放）。

### 6.2 表字段（复述自领域模型）

```sql
-- embeddings (metadata only if external vector store is used)
id uuid PRIMARY KEY
chunk_id uuid REFERENCES document_chunks(id)
model text          -- e.g. openai/text-embedding-3-small
dimension int
vector vector NULL  -- 仅 pgvector 驱动使用
quality_score numeric
created_at timestamptz
updated_at timestamptz
```

### 6.3 质量与多版本

- `quality_score`：来源于离线评测或在线反馈加权；
- 支持同一 `chunk_id` 多条 embedding（不同 `model`/`version_tag`）。

---

## 7. 向量存储适配（VectorStore Adapter）

### 7.1 驱动抽象

```go
type VectorStore interface {
    Upsert(ctx context.Context, points []Point) error
    Query(ctx context.Context, q Query) ([]ScoredPoint, error)
    DeleteByChunkIDs(ctx context.Context, chunkIDs []uuid.UUID) error
    Health(ctx context.Context) error
}

type Point struct {
    ID        string            // usually chunk_id or embedding_id
    Vector    []float32         // optional if external store manages it
    Metadata  map[string]any    // tenant_id, space_id, version_id, tags...
    Namespace string            // collection / partition
}
```

### 7.2 集合命名与分片

- 命名：`${tenant}_${space}_v${version_major}`（或统一 `powerx_kb_${tenant}` + 标签过滤）；
- 大规模场景建议：**按租户或 Space 分集合**，避免爆表；
- HNSW/IVF 参数：通过 `VectorStore.settings` 下发（M, efConstruction, nprobe…）。

### 7.3 支持驱动

- `qdrant`: `endpoint`, `collection_prefix`, `payload_index=true`；
- `pgvector`: `dsn`, `schema`, `opclass ivfflat/hnsw`, `lists/ef`；
- `milvus`: `endpoint`, `index_type HNSW/IVF_FLAT`, `metric COSINE/L2`。

---

## 8. Hybrid 检索与重排

### 8.1 检索流程

1. **候选召回**：

   - 语义召回：向量 Top-K（可按 Space/Tag 过滤）；
   - 关键词召回：BM25/tsvector/OpenSearch；
2. **融合**：Score 归一化后加权（`alpha` 可按 Space 配置）；
3. **重排**：Cross-Encoder/LLM-Reranker（仅 Top-`k_rerank`）；
4. **过滤与权限**：按租户/标签敏感级别过滤，输出 `ChunkView`。

### 8.2 配置示例

```yaml
retrieval:
  hybrid:
    alpha: 0.65              # 向量权重
    topk_vec: 50
    topk_kw: 50
    rerank:
      enabled: true
      provider: "cross-encoder/ms-marco-MiniLM-L-6-v2"
      topk: 20
```

---

## 9. 缓存与加速

- **查询缓存**：Redis 缓存 (query_hash → [chunk_id...])，TTL=300s；
- **热文档缓存**：将解析后的 `chunks.json` 存对象存储，Admin 可一键预热；
- **二级索引**：开启 `enable_inverted_index` 的空间使用 OpenSearch 存关键词索引。

> 以上与原文一致，但明确了键设计与 TTL 策略，便于落地实现与观测。

---

## 10. 配置范式（config.yaml）

```yaml
vector_store:
  default: qdrant
  drivers:
    qdrant:
      endpoint: http://localhost:6333
      collection_prefix: powerx_kb_
      payload_index: true
    pgvector:
      dsn: postgres://user:pass@host:5432/powerx
      schema: knowledge_index
      opclass: ivfflat
      lists: 100
    milvus:
      endpoint: milvus://127.0.0.1:19530
      index_type: HNSW
      metric: COSINE

embedding:
  default_model: openai/text-embedding-3-small
  providers:
    openai:
      api_key: ${OPENAI_API_KEY}
    bge:
      endpoint: http://embedder:8080
      model: bge-base-en

chunker:
  default:
    window: 512
    overlap: 64
  faq:
    by_row: true

retrieval:
  hybrid:
    alpha: 0.65
    topk_vec: 50
    topk_kw: 50
    rerank:
      enabled: true
      provider: cross-encoder/ms-marco-MiniLM-L-6-v2
      topk: 20
```

---

## 11. 监控与可观测性

- **指标（Prometheus）**

  - `kb_index_latency_seconds{step=Parse|Embed|Persist}`
  - `kb_vectorstore_qps{driver=...}`、`kb_rerank_latency_seconds`
  - `kb_task_retries_total{step=...}`、`kb_dlq_total`
- **日志（结构化）**

  - 记录 idempotency_key、checksum、外部依赖耗时；
- **分布式追踪（OpenTelemetry）**

  - 跨服务 trace：Upload → Parse → Embed → Upsert → Query → Rerank。

---

## 12. 数据保留与清理（Lifecycle）

- **版本保留**：按空间配置保留最近 `N` 个版本，超出归档或清理；
- **软删除**：`deleted_at` 标记后进入清理计划：

  - 删除外部向量（`VectorStore.DeleteByChunkIDs`）；
  - 失效缓存（Redis/ObjectStore JSON）；
  - 审计记录保留（按企业合规要求）。
- **导出备份**：打包文档 + 解析产物 + Registry 快照，便于跨环境迁移与回归测试。

> 该策略与你原文描述一致，这里进一步列出“向量清理顺序”以保证外部库与 Registry 的一致性。

---

## 13. 与 Workflow / Agent 的衔接

- **Workflow**：在索引完成事件后（`knowledge.document.indexed`），可触发

  - 「知识上线校验」→ 自动样本集评测；
  - 「缓存预热」→ 预跑关键查询；
- **Agent**：检索服务提供「召回解释」字段（命中 chunk、相似度、重排得分、来源），用于调试与 A/B。

---

## 14. 风险与对策（索引侧）

| 风险       | 影响     | 对策                            |
| -------- | ------ | ----------------------------- |
| 外部向量库不可用 | 召回失败   | 自动降级到 pgvector（若配置）、快速健康探测与熔断 |
| 模型变更导致漂移 | 召回不一致  | 双写/双检索灰度期 + Registry 批量回放工具   |
| 大租户数据倾斜  | 索引扩建困难 | 按租户/空间分集合 + HNSW 参数按集合级配置     |
| 速率限制     | 失败/延迟  | 供应商级节流 + 批量嵌入 + 指数退避重试        |

---

## 15. 运维手册（摘要）

- **常用操作**

  - 重建某文档版本：`px kb reindex --document <id> --version <no>`
  - 迁移向量库：`px kb migrate --from qdrant --to milvus --space <id>`
  - 预热空间：`px kb warmup --space <id> --queries queries.txt`
- **排障清单**

  - 检查任务面板与 DLQ；
  - `VectorStore.Health()`；
  - 对比 Registry 与外部集合数量差异；
  - 采样重放查询并对比重排前后命中列表。
