# Data Model — Knowledge Space Provisioning & Lifecycle Governance

## KnowledgeSpace
- **Identifiers**: `space_id (UUID, immutable)`, `tenant_id`, `space_name` (unique per tenant)
- **Core fields**: `department_code`, `status (draft|active|pending_iam|retired)`, `quota_cpu`, `quota_storage_gb`, `policy_template_version_id`, `feature_flags[]`, `retire_at`, `retention_expires_at`
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

> 说明：业务侧通过 `VectorStore` 抽象写入；当 driver 选择 `pgvector` 时，默认落表为 `public.knowledge_vectors`。

### knowledge_vectors

- **PK**: `(space_uuid, chunk_uuid)`
- **核心字段**:
  - `embedding vector(1536)`（维度与模型配置一致）
  - `metadata jsonb`（包含 `source_uri/format/provenance/anchors` 等）
  - `updated_at timestamptz`
- **索引建议**:
  - `space_uuid` btree
  - `embedding` 近邻索引（`ivfflat` 或 `hnsw`，按环境策略）

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
  - `content text`：用于 FTS/BM25
  - `metadata jsonb`：`provenance/anchors/time_fields/structured_fields/...`
  - `created_at / updated_at`
- **索引建议**:
  - `to_tsvector(content)` 的 GIN（FTS）
  - `metadata` 的 GIN（jsonb_path_ops）
  - `(space_uuid, kind)` btree

### knowledge_chunk_links（可选）

- **PK**: `(space_uuid, src_chunk_uuid, dst_chunk_uuid, rel_type)`
- **字段**:
  - `rel_type`：`parent/next/prev/derived_from/...`
  - `props jsonb`
  - `created_at`
