# Feature Specification: Knowledge Space Provisioning & Lifecycle Governance

**Feature Branch**: `[011-docs-use-cases]`  
**Created**: 2025-11-12  
**Status**: Draft  
**Input**: User description: "请根据文件夹docs/use_cases/_from_hub/SCN-KNOWLEDGE-SPACE-001下的用例场景文档，生成可开发的spec相关文档"

## Clarifications

### Session 2025-11-12

- Q: 知识空间在命名和分配时需要满足怎样的唯一性范围，才能确保配额治理与审计追踪准确？ → A: 名称仅需在同一租户内唯一，不同租户可重复
- Q: 当知识空间被退役时，系统应如何处理其已生成的 chunk/向量/图谱等内容，以兼顾合规留存与存储成本？ → A: 以归档只读方式保留 13 个月后再删除

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Web-admin provisioning workspace (Priority: P1)

Knowledge platform admins use the Nuxt 4 (Vue 3) Web Admin console to create a knowledge space through a guided wizard that collects tenant/department metadata, quota, policy templates, IAM roles, and alert subscriptions. The UI surfaces validations inline, displays SLA timers, and exposes an audit-ready summary before submission so operators can complete provisioning without resorting to CLI calls.

**Why this priority**: The admin portal is the primary touchpoint for Ops teams; a well-designed flow reduces configuration errors, accelerates onboarding, and ensures policy enforcement is transparent to non-technical users.

**Independent Test**: Navigate the Web Admin provisioning page, populate each step with sample data, observe inline validation + live quota checks, submit, and confirm the success banner includes SLA timing, audit tokens, and CTA links to ingestion.

**Acceptance Scenarios**:

1. **Given** an admin opens the “Create Knowledge Space” wizard, **When** they complete Step 1 (tenant/department) and Step 2 (policy/feature template) with valid entries, **Then** the UI shows a running SLA indicator (<120s) and unlocks Step 3 (quota + alert routing) with context-sensitive defaults.
2. **Given** the admin enters a space name already used within the tenant, **When** they attempt to proceed, **Then** the UI displays an inline error, highlights the conflicting row, and links to the existing space record without reloading the page.
3. **Given** IAM sync confirmation has not arrived within 90 seconds, **When** the review step is reached, **Then** the wizard shows a “Pending IAM” badge, explains the block reason, and generates an automatic follow-up task while disabling the “Launch ingestion” action.
4. **Given** the admin tries to disable mandatory security templates, **When** they toggle the switch, **Then** the UI requires dual confirmation + justification text and records the choice in the audit preview pane.

---

### User Story 2 - Multi-format ingestion baseline (Priority: P2)

Knowledge engineers ingest long PDFs, Markdown handbooks, Excel/CSV tables, and partner APIs through a single orchestrator that performs OCR, chunking, embedding, masking, and graph writes with automatic retries and coverage dashboards.

**Why this priority**: High-quality ingestion is required to reach the promised chunk coverage ≥95% and embedding success 100%, directly affecting agent answer quality.

**Independent Test**: Trigger ingestion jobs for mixed sample files using sandbox credentials, observe orchestrator outputs (chunk counts, masking status, vector + graph writes), and verify metrics without enabling fusion or feedback flows.

**Acceptance Scenarios**:

1. **Given** uploaded PDF/Markdown/Table/API sources, **When** the orchestrator runs the default pipeline, **Then** it completes within four hours, produces linked chunk/vector/graph artifacts, and reports ≥95% coverage plus 100% embedding success.
2. **Given** repeated OCR or API failures, **When** retries exceed the configured threshold, **Then** the job raises an incident event, pauses downstream publication, and enqueues manual review instructions.
3. **Given** sensitive fields detected during structured ingestion, **When** masking rules flag a violation, **Then** ingestion halts that dataset, logs the blocking reason, and leaves unaffected datasets progressing.

---

### User Story 3 - Multi-source fusion strategy management (Priority: P2)

Fusion operators combine long-doc, structured, and API outputs into a governed retrieval strategy that blends lexical, vector, and graph constraints while supporting versioning, rollback, and automatic degradation when sources fail.

**Why this priority**: Mixed-source retrieval is what drives the 15% accuracy lift promised to customers; without configurable strategies the ingestion outputs remain underutilized.

**Independent Test**: Deploy a fusion policy referencing sample corpora, run benchmark queries such as “供应商是否超限,” and measure recall/precision while simulating an upstream API outage to confirm rollback and alerting.

**Acceptance Scenarios**:

1. **Given** an existing knowledge space, **When** an operator publishes a new fusion strategy, **Then** the system versions the weights, enforces BM25+vector+graph constraints, and records the change for rollback within five minutes.
2. **Given** the real-time API portion of the fusion pipeline fails, **When** failure thresholds are hit, **Then** the strategy automatically downgrades to cached data, emits `fusion.source.failed` telemetry, and notifies operations.
3. **Given** conflicting results across sources, **When** the strategy engine detects inconsistent quota values, **Then** it routes the record into a conflict queue and prevents the inaccurate facts from reaching retrieval indexes.

---

### User Story 4 - Feedback-driven reprocessing & hot updates (Priority: P3)

Agent operators submit feedback whenever answers look stale or inaccurate, triggering a traceable loop that scores quality, reprocesses affected chunks, and hot-updates indexes and graph nodes within 24 hours.

**Why this priority**: Closing the loop maintains trust in the knowledge space and enforces compliance (PII handling, audit replay) promised in the scenario documentation.

**Independent Test**: Inject synthetic “answer incorrect” feedback referencing known chunks, monitor creation of reprocess jobs, confirm audit entries, and ensure updated content is live before the 24-hour SLA without needing new ingestions.

**Acceptance Scenarios**:

1. **Given** a submitted feedback item tied to specific chunk IDs, **When** quality scoring completes, **Then** the pipeline schedules reprocessing tasks and surfaces SLA countdown plus responsible owner in the dashboard.
2. **Given** reprocessing succeeds, **When** new chunks and embeddings are approved, **Then** vector, keyword, and graph indexes hot-swap to the latest version with less than five minutes of read-side cache inconsistency.
3. **Given** reprocessing fails repeatedly, **When** retries are exhausted, **Then** the system rolls back to the previous content version, keeps the unresolved feedback open, and escalates via PagerDuty with an attached audit ID.

---

- Concurrent space creation for the same tenant must detect quota conflicts and serialize provisioning to avoid double-allocation; the UI should lock the wizard and surface a toast when another admin is mid-creation.
- Import payloads referencing unsupported formats (e.g., password-protected PDFs) should fail fast with actionable remediation guidance.
- Structured ingestion must block uploads that contain confidential identifiers the masking policy cannot cover.
- Fusion strategies referencing deprecated pipelines should not publish; operators must be prompted to re-link compatible sources.
- Feedback submitted on deleted spaces must be rejected with guidance to reassign or restore the source space.
- Bulk reprocessing triggered by spikes (>50 feedback/hour) should throttle job creation to protect shared GPU/OCR capacity and show banner alerts in the Web Admin dashboard.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The platform MUST allow authorized admins to create, update, and retire knowledge spaces with enforced SLA ≤2 minutes from submission to activation while enforcing a 13-month read-only retention for retired assets.
- **FR-002**: Space creation MUST auto-apply default RAG, graph, masking, retention, and alerting templates; disabling any template requires an explicit approval workflow and audit entry.
- **FR-003**: Provisioning MUST validate tenant quotas, per-tenant naming uniqueness, and configuration conflicts atomically, rejecting and rolling back partial writes upon violation.
- **FR-004**: IAM role synchronization MUST complete with ≥99.5% success; unresolved sync tasks must block ingestion and emit operational alerts.
- **FR-005**: Every provisioning, policy change, and approval action MUST write to the audit stream with actor, payload hash, template versions, and rollback tokens.
- **FR-006**: The ingestion orchestrator MUST accept PDF/Markdown/Excel/CSV/API inputs, perform OCR + chunking + embedding + masking + graph linking, and finish the first ingestion cycle within four hours.
- **FR-007**: Long-document ingestion MUST deterministically produce dual-granularity chunks (≈800-token semantic summaries + ≈300-token paragraphs) with ≥95% coverage, 100% embedding success, explicit provenance (doc + page), and automated validation reports surfaced to operators.
- **FR-008**: Structured ingestion MUST detect schema elements (keys, timestamp, enumerations), enforce masking coverage 100%, and block publication when sensitivity checks fail.
- **FR-009**: Every ingestion job MUST provide retries (up to three automatic attempts) with exponential backoff and emit `knowledge.ingestion.*` events for success, failure, and manual review states.
- **FR-010**: Fusion pipelines MUST support configurable combinations of lexical, vector, and graph constraints, allow operators to version weights, and guarantee rollback to any prior version within five minutes.
- **FR-011**: The platform MUST monitor external API dependencies used in fusion, degrade gracefully to cached data on sustained failures, and notify SRE plus the tenant admin.
- **FR-012**: Feedback submission MUST capture the answer context, chunk/tool trace, and severity, then trigger quality scoring and reprocess jobs with SLA monitoring (≤24 hours to completion).
- **FR-013**: Hot index updates MUST swap vector, keyword, and graph artifacts atomically, ensuring no more than five minutes of stale responses during deployment.
- **FR-014**: All flows (provisioning, ingestion, fusion, feedback) MUST expose metrics to Grafana dashboards (`Knowledge Space`, `fusion-pipeline`, `feedback-loop`) and export summarized JSON under `reports/_state`.

### Key Entities *(include if feature involves data)*

- **Knowledge Space**: Logical container tying tenant, department, quotas, policy templates, feature flags, and status (`draft`, `active`, `pending-iam`, `retired`); names must be unique within the tenant scope while system IDs remain global, and retired spaces keep read-only artifacts for 13 months before purge.
- **Policy Template Version**: Snapshot of security, ingestion, and governance rules applied to a space; records template source, reviewers, and rollback token.
- **Ingestion Job**: Execution record for a specific source file or API, storing source metadata, processing stages, retry count, chunk/embedding statistics, and blocking reasons.
- **Fusion Strategy**: Versioned configuration describing data sources, weighting, reranking options, and degradation rules plus associated benchmark metrics.
- **Feedback Case**: User- or agent-submitted issue referencing answer IDs, related chunks, severity, SLA deadline, reprocessing status, and linked audit trail.
- **Artifact Bundle**: Snapshot tying chunk files, vector embeddings, graph nodes, and masking manifests for a single ingestion job; stores storage URIs, hash, size, and retention status so orchestrators can roll forward/backward safely.

## Assumptions

- Front-end UX for admins and operators already exists; this feature delivers backend service surfaces and contracts consumed by those experiences.
- Required IAM, audit, and monitoring services are available and can accept new events/metrics without additional provisioning work in this feature.
- Sample corpora (long PDF, structured expense sheets, test APIs) are available in non-production environments for automated validation.
- Feature flags (`knowledge-space-v1`, `knowledge-ingestion`, `structured-ingestion`, `fusion.pipeline`, `feedback.loop`) will be enabled per-tenant via existing configuration management.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 95% of knowledge space provisioning requests finish (success or actionable rejection) within 2 minutes, with 0 orphaned IAM records.
- **SC-002**: First-pass ingestion of mixed modalities completes within 4 hours with ≥95% chunk coverage, 100% embedding success, and 100% masking enforcement on structured data.
- **SC-003**: Fusion strategies deliver at least a 15% improvement in retrieval accuracy for benchmark questions versus single-source baselines, and rollback completes in under 5 minutes when triggered.
- **SC-004**: Feedback cases close (reprocessed + hot-updated) within 24 hours in ≥95% of instances; unresolved cases automatically escalate after SLA breach.
- **SC-005**: All critical pipelines emit health metrics and alerts with <5 minutes detection time, and there are zero gaps in audit trails for provisioning, ingestion, fusion, or feedback events.
- **SC-006**: Compliance metrics (masking coverage, IAM sync success, audit completeness) stay at 100% for production tenants during pilot rollout.
