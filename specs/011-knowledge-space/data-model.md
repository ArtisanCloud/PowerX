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
