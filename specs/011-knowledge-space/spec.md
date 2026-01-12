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

### User Story 5 - QA orchestrator-ready cross-space reasoning surfaces (Priority: P1)

QA Orchestrator and Agent Dialogue services (per `SCN-KNOWLEDGE-QA-REASON-001`) consume knowledge-space metadata to pick the right spaces, stream conversation memory, assemble reasoning plans that mix chunks + tools, and enforce security checks before responding to end users. Knowledge spaces must therefore expose cross-space retrieval readiness, citation deltas, and toolchain contracts with ≤2s SLA so QA flows can stay trustworthy across multiple tenants.

**Why this priority**: Intelligent QA is the downstream consumer that validates the value of every knowledge space. Without first-class orchestration hooks, the scenario’s KPIs (≤2s retrieval, ≥95% citation coverage, ≥99% tool success, 24h negative-feedback closure) cannot be met even if ingestion quality is high.

**Independent Test**: Point the sandbox QA Orchestrator at two active knowledge spaces, call the new `POST /knowledge-spaces/qa/retrieval-plan` API with labeled intents, watch it return within 2 seconds with citation scores + degrade reasons, trigger a follow-up request to verify conversation-memory deltas, and force a tool failure to confirm failover + audit events fire before the Agent completes the answer.

**Acceptance Scenarios**:

1. **Given** a QA Orchestrator request carrying `intent`, `domain_tags`, and desired latency, **When** the knowledge-space service computes the cross-space retrieval plan, **Then** it returns ≤2s with ≥95% citation coverage, identifies at least two eligible spaces, and emits `qa.retrieval.plan` telemetry that records degrade reasons for any skipped space.
2. **Given** a multi-turn follow-up, **When** the Agent Dialogue service queries `/knowledge-spaces/qa/memory-snapshot`, **Then** it receives citation deltas mapped to chunk IDs + knowledge space IDs, with an audit pointer so security reviewers can replay the reasoning chain.
3. **Given** the reasoning plan references SQL/REST tools registered for a knowledge space, **When** the QA Orchestrator invokes the new toolchain contract, **Then** the service resolves tool metadata, enforces IAM scopes, logs each step under `audit.reasoning_steps`, and automatically fails over to cached data if real-time calls fall below the 99% success target.
4. **Given** sensitive fields or unauthorized spaces are detected during plan generation, **When** the compliance hooks run, **Then** the request is blocked with `security.access.denied`, a masked preview is returned, an incident alert is raised, and the original QA session automatically links to the audit ID for follow-up.

---

### User Story 6 - Knowledge update, decay guard, and tenant release operations (Priority: P1)

Knowledge governance operators run the continuous-update control room described in `docs/use_cases/_from_hub/SCN-KNOWLEDGE-UPDATE-001/SCN-KNOWLEDGE-UPDATE-001.md`: they launch delta sync jobs, approve diffs, trigger event hotfixes, monitor decay scans, and orchestrate tenant-level gray releases with full auditability and SLA-backed telemetry.

**Why this priority**: Without delta/feedback/event/decay/release loops, knowledge spaces drift from reality even if provisioning + ingestion are solid. The scenario mandates ≤30m delta pipelines with ≥98% diff accuracy, ≤5m event refresh, 100% decay coverage, and auditable tenant releases, all of which unlock safe incremental updates for regulated tenants.

**Independent Test**: Execute `scripts/ops/knowledge-delta-job.mjs --tenant demo-retail` to generate a delta package, review the diff + approval UI, promote to pilot tenants, then push a simulated regulation-update event into the bus and confirm ≤5m hotfix latency. Immediately follow with `scripts/ops/knowledge-decay-scan.mjs` to produce decay tasks, resolve one false positive within 10 minutes, and finally promote the new version through the gray-release UI/CLI while verifying `backend/reports/_state/knowledge-*.json` snapshots.

**Scenario Inputs & Guardrails**:

- `SCN-KNOWLEDGE-UPDATE-SYNC-001.md` (delta/approval/version) → requires `PX_KNOWLEDGE_DELTA_SYNC`, `PX_KNOWLEDGE_VERSIONED_STORAGE`, and partial-release controls mapped to HTTP+gRPC contracts.
- `SCN-KNOWLEDGE-UPDATE-EVENT-001.md` (event hotfix/agent notify) → enforces event signatures, idempotent keys, ≤5m latency, and `PX_AGENT_WEIGHT_REFRESH` gating.
- `SCN-KNOWLEDGE-UPDATE-DECAY-001.md` (decay/gap watchdog) → drives schedule frequency, severity thresholds, ≤10m restore, and `PX_KNOWLEDGE_DECAY_GUARD` / `PX_KNOWLEDGE_GAP_ALERT` flags.
- `SCN-KNOWLEDGE-UPDATE-TENANT-001.md` (tenant-aware gray release) → mandates `PX_KNOWLEDGE_GRAY_RELEASE`, `PX_TENANT_RELEASE_MATRIX`, `PX_KNOWLEDGE_RELEASE_GUARD`, version drift ≤1 release, 5-minute rollback SLA, and per-tenant audit exports sourced from `tenant_release_matrix.yaml` + `release_guardrails.md`.
- All loops must write telemetry snapshots into `backend/reports/_state/knowledge-{delta,event,decay}.json` and aggregate into `reports/_state/knowledge-update.json` per constitution.

**Implementation Notes**: Delta + event flows reuse the shared multi-driver vectorstore abstraction defined in `backend/pkg/corex/db/persistence/vectorstore` instead of bespoke embedding clients, so pgvector/milvus/pinecone drivers stay centrally configurable. Task orchestration reuses the existing approval-center connectors, audit-ledger client, and `task-center` integration for decay gap workloads.

**Acceptance Scenarios**:

1. **Given** a delta job referencing updated PDFs + API sources, **When** operators run the approval flow, **Then** the system enforces ≤30 minute detect→publish SLA, emits `knowledge.delta.{sla,diff_accuracy,partial_release}` (target ≥98% diff accuracy), stores partial-release + rollback tokens, reuses the vectorstore driver registry for embedding comparisons, and writes audit IDs matching `SCN-KNOWLEDGE-UPDATE-SYNC-001`.
2. **Given** a policy-change event lands on the bus, **When** the event-hotfix handler matches `backend/config/knowledge/event_hotfix_policies.yaml`, **Then** it completes the refresh within five minutes, validates payload signatures, produces `knowledge.event.{latency,idempotent_skips}` for duplicates, refreshes Agent weights through the notifier component, and records the hotfix in `backend/reports/_state/knowledge-event.json`.
3. **Given** the nightly decay scan identifies low-quality or empty topics, **When** the decay guard triages them, **Then** it generates restoration tasks in `task-center` with SLA ≤7 days, flags potential false positives, allows a one-click restore in ≤10 minutes, and exports `knowledge.decay.detected`, `knowledge.decay.false_positive`, and `knowledge.gap.backlog` metrics as specified by `SCN-KNOWLEDGE-UPDATE-DECAY-001.md`.
4. **Given** new content needs tenant-aware rollout, **When** operators configure `backend/config/knowledge/tenant_release_matrix.yaml` and start a gray release, **Then** the release controller promotes pilots, pauses automatically when metrics violate guardrails, rolls back affected tenants inside five minutes, and emits auditable `knowledge.release.gray_state` entries covering approvals + rollback reasons per `SCN-KNOWLEDGE-UPDATE-TENANT-001.md`.

---

- Concurrent space creation for the same tenant must detect quota conflicts and serialize provisioning to avoid double-allocation; the UI should lock the wizard and surface a toast when another admin is mid-creation.
- Import payloads referencing unsupported formats (e.g., password-protected PDFs) should fail fast with actionable remediation guidance.
- Structured ingestion must block uploads that contain confidential identifiers the masking policy cannot cover.
- Fusion strategies referencing deprecated pipelines should not publish; operators must be prompted to re-link compatible sources.
- Feedback submitted on deleted spaces must be rejected with guidance to reassign or restore the source space.
- Bulk reprocessing triggered by spikes (>50 feedback/hour) should throttle job creation to protect shared GPU/OCR capacity and show banner alerts in the Web Admin dashboard.
- Cross-space retrieval plan requests must degrade gracefully when a knowledge space is offline or unauthorized by returning structured `degrade_reason` codes, emitting `qa.degrade.count`, and blocking propagation to QA Orchestrator until compliance gates pass.

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
- **FR-015**: Knowledge spaces MUST expose a QA Orchestrator bridge (HTTP + gRPC) that accepts intent/tag payloads, returns cross-space retrieval plans within ≤2 seconds, annotates citation coverage (goal ≥95%), and encodes degrade reasons aligned with `SCN-KNOWLEDGE-QA-RETRIEVE-001`.
- **FR-016**: The service MUST provide conversation-memory snapshots/deltas per tenant + knowledge space so Agent Dialogue flows can reuse citations, highlight differences, and persist audits as outlined in `SCN-KNOWLEDGE-QA-CONTEXT-001`.
- **FR-017**: Toolchain metadata (SQL/REST/rule engines) tied to each knowledge space MUST be discoverable and invocable via the bridge, including failover policies, audit identifiers, and cached outputs to satisfy `SCN-KNOWLEDGE-QA-TOOL-001`.
- **FR-018**: Each QA Orchestrator interaction MUST call IAM access checks and sensitive-data detectors before responding, block unauthorized requests, and write `audit.reasoning_steps` + `audit.security` records that comply with `SCN-KNOWLEDGE-QA-COMPLIANCE-001`.
- **FR-019**: QA-feedback events sourced from Agent sessions MUST funnel into the existing feedback loop, keeping the ≤24h SLA, tagging the originating QA session, and producing `qa.feedback.loop_time` metrics per `SCN-KNOWLEDGE-QA-FEEDBACK-001`.
- **FR-020**: Delta sync + version governance flows MUST implement the APIs/events enumerated in `docs/use_cases/_from_hub/SCN-KNOWLEDGE-UPDATE-001/SCN-KNOWLEDGE-UPDATE-SYNC-001.md`—including schedulable `POST /knowledge/delta/jobs`, diff reports, approval adapters, partial release, and rollback endpoints—while ensuring ≤30m SLA, ≥98% diff accuracy, and audit-aligned rollback tokens.
- **FR-021**: Event hotfix orchestration per `SCN-KNOWLEDGE-UPDATE-EVENT-001.md` MUST subscribe to `knowledge.event.received`, validate signatures, apply playbook policies, refresh indexes/Agent weights within ≤5 minutes, and expose HTTP + gRPC transports plus CLI tooling for replay along with idempotent skip tracking.
- **FR-022**: Decay/gap detection described in `SCN-KNOWLEDGE-UPDATE-DECAY-001.md` MUST run automated scans (cron + on-demand), classify severity, spawn restoration tasks with ≤7-day SLA, support ≤10-minute false-positive recovery, and guard rail multi-tenant visibility with audit logging.
- **FR-023**: Tenant release governance per `SCN-KNOWLEDGE-UPDATE-TENANT-001.md` MUST manage `tenant_release_matrix.yaml`, pilot selection, automated expansion, failure-induced rollback (<5 minutes), and cross-tenant audit/export capabilities accessible via HTTP/gRPC + CLI + Web Admin surfaces.
- **FR-024**: All knowledge-update flows (delta, feedback, event, decay, tenant release) MUST emit metrics (`knowledge.delta.*`, `knowledge.feedback.*`, `knowledge.event.*`, `knowledge.decay.*`, `knowledge.release.*`) into OpenTelemetry, Grafana dashboards, and JSON exports (`backend/reports/_state/knowledge-{delta,feedback,event,decay,release}.json` + aggregated `knowledge-update.json`).

### Scene & Strategy Bundles (RAG Productization)

This feature MUST implement the scene-driven strategy selection model described in:
- `docs/plan/AI_engineering/knowledge/rag.md`
- `docs/plan/AI_engineering/knowledge/rag_scene_strategy_mode.md`

Definitions:
- **Scene** (L1 selection): the knowledge base category + typical query intents (e.g., SOP, contract, research, ledger, SQL/KG).
- **Strategy bundle** (L2 selection): a versioned combination of `IngestionProfile + IndexProfile + RAGProfile + Guardrails`.

Non-goal: Do **not** expose a full Cartesian product of “scenes × all strategies”. Only show strategy bundles that match the scene’s index/asset prerequisites.

- **FR-025**: The Web Admin MUST offer a unified, guided entry that supports two-level selection: `Scene → Strategy bundle`, plus a “Custom scene (expert)” option that can unlock all modules with dependency validation.
- **FR-026**: The platform MUST enforce strategy prerequisites before allowing activation/publish (e.g., KG bundles require KG indexes/tables; high-accuracy bundles require sparse index + evidence guardrails), and MUST surface actionable remediation in UI.
- **FR-027**: Each scene MUST map to a biased (non-full) set of strategy bundles and strategy modules as defined in `docs/plan/AI_engineering/knowledge/rag_scene_strategy_mode.md`, including:
  - KG as the default for the “SQL/config/dependency” scene (KG-strong), and optional KG-lite for contract scenarios.
  - Contract/quote scenes default to evidence-first (sparse-heavy + CRAG + must-cite + time-aware).
- **FR-028**: Ingestion profiles MUST support configurable chunking parameters (e.g., chunk size, overlap/delta, separators) with scene defaults and safe bounds, and MUST capture provenance fields required by retrieval citations.
- **FR-029**: `make db-migrate` MUST provision knowledge-space persistence prerequisites in PostgreSQL: when `knowledge_space.vector_store.driver=pgvector`, it MUST ensure `pgvector` extension + `knowledge_vectors` table (and required indexes) exist; it MUST also provision minimal KG assist tables (`knowledge_kg_nodes`, `knowledge_kg_edges`) idempotently so KG-enabled strategy bundles can be activated without manual SQL.
- **FR-030**: The system MUST fail fast with actionable errors when migrations cannot create required extensions/tables (e.g. missing `CREATE EXTENSION vector` privilege), and MUST skip pgvector-only migrations when non-pgvector drivers (Milvus/Pinecone) are configured.
- **FR-031**: The system MUST map `scene_strategy_catalog.yaml` index prerequisites to concrete storage readiness checks. When a scene/strategy enables `index.sparse`/`index.hier`/`index.structured_fields` using the Postgres-backed implementation, `make db-migrate` MUST provision the corresponding assist tables (e.g. `knowledge_chunks`, `knowledge_chunk_links`) and indexes idempotently.

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
- Feature flags (`knowledge-space-v1`, `knowledge-ingestion`, `structured-ingestion`, `fusion.pipeline`, `feedback.loop`, `PX_KNOWLEDGE_DELTA_SYNC`, `PX_KNOWLEDGE_FEEDBACK_LOOP`, `PX_KNOWLEDGE_EVENT_HOTFIX`, `PX_KNOWLEDGE_DECAY_GUARD`, `PX_KNOWLEDGE_GRAY_RELEASE`) will be enabled per-tenant via existing configuration management with rollout plans captured in `tenant_release_matrix.yaml`.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 95% of knowledge space provisioning requests finish (success or actionable rejection) within 2 minutes, with 0 orphaned IAM records.
- **SC-002**: First-pass ingestion of mixed modalities completes within 4 hours with ≥95% chunk coverage, 100% embedding success, and 100% masking enforcement on structured data.
- **SC-003**: Fusion strategies deliver at least a 15% improvement in retrieval accuracy for benchmark questions versus single-source baselines, and rollback completes in under 5 minutes when triggered.
- **SC-004**: Feedback cases close (reprocessed + hot-updated) within 24 hours in ≥95% of instances; unresolved cases automatically escalate after SLA breach.
- **SC-005**: All critical pipelines emit health metrics and alerts with <5 minutes detection time, and there are zero gaps in audit trails for provisioning, ingestion, fusion, or feedback events.
- **SC-006**: Compliance metrics (masking coverage, IAM sync success, audit completeness) stay at 100% for production tenants during pilot rollout.
- **SC-007**: QA Orchestrator calls spanning at least two knowledge spaces complete cross-space retrieval plans in ≤2 seconds, maintain ≥95% citation coverage, keep real-time tool success ≥99%, and auto-close ≥95% of QA-sourced feedback within 24 hours.
- **SC-008**: Knowledge delta jobs detected via `scripts/ops/knowledge-delta-job.mjs` publish within ≤30 minutes in ≥95% of cases, diff accuracy stays ≥98%, and rollback from any failed release completes in ≤5 minutes with full audit coverage.
- **SC-009**: Event hotfixes sourced from `knowledge.event.received` complete refresh + Agent notifications within ≤5 minutes, idempotent skips are recorded for 100% of duplicate payloads, and `knowledge.event.retry_count` never exceeds three without escalation.
- **SC-010**: Decay scans achieve 100% coverage of active knowledge spaces, detect low-quality/empty segments with ≥90% precision, auto-create restoration tasks with SLA ≤7 days, and resolve false positives or restores within 10 minutes.
- **SC-011**: Tenant gray releases apply policies from `tenant_release_matrix.yaml`, keep version drift ≤1 release across tenants, roll back failing batches in ≤5 minutes, and expose `knowledge.release.gray_state` / `knowledge-release.json` snapshots that auditors can reconcile without manual reconstruction.
