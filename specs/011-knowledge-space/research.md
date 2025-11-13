# Phase 0 Research — Knowledge Space Provisioning & Lifecycle Governance

## Decision 1: Tenant-scoped naming with PostgreSQL-backed metadata
- **Decision**: Persist knowledge space metadata (tenant, department, quotas, policy version, retention flags) in PostgreSQL with a composite unique index on `(tenant_id, space_name)` while keeping immutable `space_id` GUIDs for cross-service references.
- **Rationale**: Aligns with clarification that names only need to be unique within a tenant, avoids cross-tenant coupling, and leverages existing GORM models + migration tooling. Composite keys simplify auditing, quotas, and metrics joins.
- **Alternatives considered**:
  - *Global unique names*: Would complicate large enterprise onboarding where business units expect overlapping vocabulary.
  - *Redis-backed registry*: Fast but weak on transactional guarantees, making rollback and IAM sync auditing brittle.

## Decision 2: Observability + SLA tracking via Grafana + reports/_state exports
- **Decision**: Instrument provisioning, ingestion, fusion, and feedback pipelines with OpenTelemetry metrics shipped to Grafana dashboards (`Knowledge Space`, `fusion-pipeline`, `feedback-loop`) and persist summary snapshots under `reports/_state/knowledge-spaces.json`.
- **Rationale**: Meets success criteria for <5 minute detection latency while matching existing operational tooling referenced in SCN documents. JSON exports unblock leadership reporting without new infra.
- **Alternatives considered**:
  - *Custom dashboard stack*: High effort, duplicates existing SRE workflows.
  - *Logs-only alerts*: Slower detection and weaker SLA evidence.

## Decision 3: 13-month read-only retention workflow for retired spaces
- **Decision**: When a space transitions to `retired`, mark downstream storage (chunks, vectors, graph nodes) as read-only, keep them discoverable for 13 months, then purge via scheduled batch jobs tied to audit tokens.
- **Rationale**: Mirrors UC retention expectations, balances compliance (audit replay, regulatory lookback) with storage costs, and gives predictable purging windows for ops teams.
- **Alternatives considered**:
  - *Immediate deletion*: Violates compliance + forensic requirements.
  - *Manual admin choice per space*: Adds operational variability and risk of accidental early deletion.

## Decision 4: Fusion degradation + rollback patterns
- **Decision**: Maintain strategy versions as immutable rows with weight blobs; on upstream API failure, degrade to cached data while emitting `fusion.source.failed` and allow operators to rollback within 5 minutes via CLI hooks.
- **Rationale**: Directly satisfies FR-010/FR-011 and SC-003; leverages existing CLI/automation surfaces.
- **Alternatives considered**:
  - *Automatic re-training per failure*: Slow and unnecessary for short outages.
  - *Manual reconfiguration only*: Slows MTTR and risks inconsistent rollback steps.
