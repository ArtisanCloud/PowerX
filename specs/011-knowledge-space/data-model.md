# Data Model — Knowledge Space Provisioning & Lifecycle Governance

## KnowledgeSpace
- **Identifiers**: `space_id (UUID, immutable)`, `tenant_id`, `space_name` (unique per tenant)
- **Core fields**: `department_code`, `status (draft|active|pending_iam|retired)`, `quota_cpu`, `quota_storage_gb`, `policy_template_version_id`, `strategy_package_key (A0–O)`, `feature_flags[]`, `retire_at`, `retention_expires_at`
- **Audit fields**: `created_by`, `created_at`, `updated_at`, `last_audited_at`, `audit_token`
- **Relationships**:
  - 1:N with `PolicyTemplateVersion` (one active template per space, but historical links stored)
  - 1:N with `IngestionJob` (space owns jobs)
  - 1:N with `FusionStrategyVersion`
  - 1:N with `FeedbackCase`
- **Validation rules**:
  - `(tenant_id, space_name)` unique
  - `status=retired` ⇒ `retention_expires_at = retired_at + 13 months`
  - Quotas must be positive and within tenant limits

## PolicyTemplateVersion
- **Identifiers**: `policy_template_version_id`
- **Fields**: `template_name`, `version`, `rag_profile`, `graph_profile`, `masking_profile`, `alerting_profile`, `approved_by`, `approved_at`, `rollback_token`
- **Relationships**: Linked to multiple `KnowledgeSpace` records via `policy_template_version_id`
- **Validation**: Template must be approved before assignment; immutable payload once published

## IngestionJob
- **Identifiers**: `job_id`, `space_id`, `source_id`
- **Fields**: `source_type (pdf|markdown|table|api)`, `status (pending|running|retrying|paused|completed|failed|blocked)`, `retry_count`, `chunk_total`, `chunk_covered_pct`, `embedding_success_pct`, `masking_coverage_pct`, `error_code`, `blocked_reason`
- **Relationships**: Belongs to `KnowledgeSpace`; references `ArtifactBundle` (chunk/vector/graph URIs)
- **Validation**:
  - Automatically enforce ≤3 retries
  - `masking_coverage_pct` must equal 100 for structured sources or status becomes `blocked`

## ArtifactBundle
- **Identifiers**: `bundle_id`, `job_id`
- **Fields**: `chunk_manifest_uri`, `vector_manifest_uri`, `graph_manifest_uri`, `masking_report_uri`, `summary_chunk_count`, `paragraph_chunk_count`, `checksum`, `storage_class`, `retained_until`
- **OCR addendum (Plan B for scanned PDFs)**:
  - `ocr_page_images_uri`（可选）：逐页渲染图片清单（或目录 URI）
  - `ocr_raw_manifest_uri`（可选）：逐页 TSV/hOCR 清单（或目录 URI）
  - `ocr_searchable_pdf_uri`（可选）：OCR 后可搜索 PDF（便于下载/检索/复制，不作为 bbox 权威来源）
- **Relationships**: 1:1 with `IngestionJob`; consumed by reprocess pipeline与 rollback 工具。
- **Validation**:
  - 所有 manifest URI 必须在 MinIO/S3 验证 checksum
  - `summary_chunk_count` 与 `paragraph_chunk_count` 应匹配算法输出（≈800 / ≈300 token）
  - `retained_until` 与所属空间保留策略一致，并支持 `active|archived|purged` 状态

## FusionStrategyVersion
- **Identifiers**: `strategy_version_id`, `space_id`
- **Fields**: `label`, `bm25_weight`, `vector_weight`, `graph_constraint`, `reranker_model`, `conflict_policy`, `published_at`, `published_by`, `rollback_from_version_id`
- **Relationships**: Many per `KnowledgeSpace`; linked to benchmark metrics snapshots
- **Validation**:
  - Sum of weights normalized (0-1)
  - Must store `deployment_state (active|rollback|draft)`

## FeedbackCase
- **Identifiers**: `case_id`, `space_id`
- **Fields**: `reported_by`, `severity (low|medium|high|critical)`, `status (open|in_progress|reprocessed|escalated|closed)`, `linked_chunks[]`, `tool_trace_ref`, `sla_due_at`, `resolution_notes`
- **Relationships**: Links to `IngestionJob` (reprocess job) and `FusionStrategyVersion` if rollback triggered
- **Validation**:
  - SLA timers computed from severity (≤24h overall)
  - PII fields hashed/anonymized before persistence

## Event + Audit Entities (logical)
- **IAMSyncTask**: Tracks provisioning-time role propagation, keyed by `(space_id, iam_system)` with status + retry fields.
- **AuditTrailEntry**: Append-only log referencing `space_id`, `action`, `payload_hash`, `actor`, `timestamp`, `rollback_token`.

---

## Vector Store（pgvector）

> 说明：业务侧通过 `VectorStore` 抽象写入；当 driver 选择 `pgvector` 时，向量表采用“全局共享 + 按维度分表”的策略（避免按 tenant/space 爆炸建表，同时支持未来切换 embedding provider/model）。

### 设计原则（强约束）

1. **space 级锁定 embedding profile**：同一 `space_uuid` 的向量必须来自同一 embedding profile（provider+model+dim），避免“同空间混模型/混空间”的检索失真。
2. **向量表按维度分表**：不同维度必须写入不同 `vector(D)` 列类型；同维度不同模型允许共用一张表，但必须通过 `space_uuid` 路由隔离。
3. **维度由系统探测/登记**：不要求管理员手工填写维度；保存/激活前必须通过一次 probe 探测得到 `dimensions`，并写入 profile 或 index registry。

### knowledge_vector_indexes（索引登记表，SSOT）

用途：记录每个 space 当前激活的 dense 向量索引落点（维度/表名/模型来源），并用于治理（清理未使用索引、回滚、审计）。

- **PK**: `id`
- **核心字段**:
  - `space_uuid uuid NOT NULL`
  - `index_key varchar(128) NOT NULL`（例如 `dense_v1_1536`）
  - `table_name varchar(128) NOT NULL`（例如 `knowledge_vectors_v1_1536`）
  - `dimensions int NOT NULL`
  - `embedding_provider varchar(64) NOT NULL`
  - `embedding_model varchar(128) NOT NULL`
  - `embedding_profile_ref varchar(128)`（引用 AI Settings 里的 profile 逻辑键，或 `{env}:{provider}:{model}`）
  - `status varchar(32)`（`creating|active|retired|failed`）
  - `created_at/updated_at`
  - `last_used_at`（用于垃圾回收）
- **索引建议**:
  - `(space_uuid, status)`、`(index_key)`、`(last_used_at)`

> 约定：一个 space 同时允许存在多个 index 记录（用于回滚/AB），但只能有一个 `status=active` 的 dense index。

### knowledge_vectors_v{N}_{D}（向量表族，pgvector）

- **命名**: `knowledge_vectors_v<N>_<D>`（例如 `knowledge_vectors_v1_1536`、`knowledge_vectors_v1_1024`）
- **PK**: `(space_uuid, chunk_uuid)`（全局共享表，通过 space_uuid 隔离）
- **核心字段**:
  - `embedding vector(D)`（D 与该表名一致）
  - `metadata jsonb`（包含 `source_uri/format/provenance/anchors` 等）
    - 必须包含：`embedding_provider`、`embedding_model`、`embedding_env`（用于审计与排查）
  - `updated_at timestamptz`
- **索引建议**:
  - `space_uuid` btree
  - `embedding` 近邻索引（`ivfflat` 或 `hnsw`，按环境策略）

### knowledge_spaces（新增/扩展字段建议）

- `embedding_profile_key`：space 绑定的 embedding profile（逻辑键）
- `active_vector_index_key`：space 当前激活的 dense index（指向 `knowledge_vector_indexes.index_key`）

> 说明：space 绑定 embedding profile 后，入库与检索都必须使用该 profile；租户级别的 active embedding profile 只作为“默认值/创建时建议”，不能直接影响已存在的 space（避免线上突变）。

---

## KG Assist Tables（最小图谱存储）

> 说明：用于 `K_kg` 等策略的“协助表”，为后续图谱抽取/回滚/审计提供持久化面。

### knowledge_kg_nodes

- **PK**: `(space_uuid, node_id)`
- **字段**:
  - `node_type`（entity/object/etc）
  - `props jsonb`（实体属性、来源、别名等）
  - `created_at / updated_at`

### knowledge_kg_edges

- **PK**: `(space_uuid, edge_id)`
- **字段**:
  - `src_node_id / dst_node_id`
  - `predicate`（关系谓词）
  - `props jsonb`
  - `created_at / updated_at`

---

## Chunk Store（用于 sparse/hier/structured）

> 说明：这是“在线检索加速”的存储面；ArtifactBundle 仍负责离线产物版本化与 lineage。

### knowledge_chunks

- **PK**: `(space_uuid, chunk_uuid)`
- **字段**:
  - `kind`：`doc_summary/section_summary/chunk/...`
  - `content text`：用于检索/引用/人工校正（真相源）
  - `metadata jsonb`：`provenance/anchors/time_fields/structured_fields/...`，其中扫描 PDF 的 provenance 推荐包含 `page_number + bbox_norm`（归一化坐标、左上原点、支持跨页多框）
  - `created_at / updated_at`
- **索引建议**:
  - `to_tsvector(content)` 的 GIN（FTS）
  - `metadata` 的 GIN（jsonb_path_ops）
  - `(space_uuid, kind)` btree

> 说明：`knowledge_vectors`（向量索引）可重建；`knowledge_chunks.content/metadata` 应作为可审计、可编辑的真相源。编辑 chunk 后需同步更新向量索引（Upsert）并记录 `edited_at/edited_by/edit_reason`。

### knowledge_chunk_links（可选）

- **PK**: `(space_uuid, src_chunk_uuid, dst_chunk_uuid, rel_type)`
- **字段**:
  - `rel_type`：`parent/next/prev/derived_from/...`
  - `props jsonb`
  - `created_at`
