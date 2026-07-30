# Data Model — Workflow & Agent Orchestration

## WorkflowDefinition
- **Fields**
  - `uuid (UUID)` — primary key
  - `tenant_uuid (UUID)` — owning tenant, required
  - `name (string)` — human readable, unique per tenant + version
  - `version (int)` — monotonic, starts at 1, composite unique with tenant/name
  - `status (enum: draft|published|archived)` — lifecycle of definition
  - `input_schema (jsonb)` — JSON schema for StartInstance input
  - `step_graph (jsonb)` — ordered/parallel topology, validated on ingest
  - `retry_policy (jsonb)` — default step retry/backoff settings
  - `compensation_policy (jsonb)` — global compensation configuration
  - `workflow_pack_key (string, nullable)` — seeded pack source, if any
  - `source_type (enum: manual|builtin_pack|plugin_pack|imported)` — definition source
  - `checksum (string, nullable)` — seed/import checksum
  - `created_by (UUID)` — user/agent who created
  - `created_at / updated_at (timestamptz)`
- **Relationships**
  - 1:N → `WorkflowInstance`
- **Rules**
  - Published definitions are immutable; updates create a new version.
  - Tenant isolation enforced (no cross-tenant lookup).

## WorkflowInstance
- **Fields**
  - `uuid (UUID)` — primary key
  - `definition_uuid (UUID)` — FK → WorkflowDefinition.uuid
  - `definition_version (int)` — immutable published version
  - `tenant_uuid (UUID)` — copied from definition
  - `agent_uuid (UUID, nullable)` — initiating or owning Agent
  - `initiator_user_uuid (UUID, nullable)` — user that started the instance
  - `state (enum: draft|running|waiting|suspended|succeeded|failed|compensating|compensated|canceled|compensation_failed)`
  - `input_context (jsonb)` — runtime inputs/variables
  - `runtime_context (jsonb)` — vars/artifacts/review/trace data shared between nodes
  - `output_context (jsonb)` — aggregated outputs
  - `trace_id (string)` — trace correlation
  - `correlation_id (string)` — external correlation
  - `sla_deadline (timestamptz)` — computed from definition
  - `started_at / completed_at (timestamptz)`
  - `last_error (text)` — final failure summary
- **Relationships**
  - 1:N → `WorkflowStepRecord`
- **Rules**
  - State transitions recorded atomically with timestamps.
  - Instances reference immutable definition version even if newer version exists.

## WorkflowStepRecord
- **Fields**
  - `id (bigserial)` — primary key
  - `instance_uuid (UUID)` — FK → WorkflowInstance.uuid
  - `step_id (string)` — matches logical step key in definition
  - `node_kind (string)` — semantic runtime adapter key
  - `node_ref (string)` — Skill, Capability, internal adapter, or workflow-local ref
  - `subject_type (enum: agent|system|human)`
  - `subject_uuid (UUID)` — Agent UUID or operator UUID when applicable
  - `grant_version (bigint)` — Tool Grant version dispatched
  - `state (enum: queued|in_progress|waiting|completed|failed|compensating|compensated)`
  - `attempt (int)` — current retry count
  - `input_mapping (jsonb)` — declared context reads
  - `output_mapping (jsonb)` — declared context writes
  - `payload_in (jsonb)` — normalized inputs sent to agent/system
  - `payload_out (jsonb)` — result payload
  - `error_code (string)` — structured failure code
  - `error_message (text)` — latest failure description
  - `started_at / finished_at (timestamptz)`
- **Relationships**
  - N:1 → WorkflowInstance
  - Optional 1:1 → `WorkflowStepCompensation`
- **Rules**
  - `(instance_uuid, step_id, attempt)` unique to track retries.
  - When `state` transitions to `completed`, `payload_out` must be immutable.

## WorkflowStepCompensation
- **Fields**
  - `id (bigserial)` — PK
  - `step_record_id (bigint)` — FK → WorkflowStepRecord
  - `state (enum: pending|executing|completed|failed)`
  - `handler (string)` — reference to compensation step definition
  - `initiated_by (enum: auto|operator)`
  - `notes (text)`
  - `created_at / updated_at (timestamptz)`
- **Rules**
  - Compensation executes strictly in reverse order of original step completion.
  - Failed compensation leaves WorkflowInstance in `compensation_failed`.

## AgentAssignment
- **Fields**
  - `id (bigserial)` — PK
  - `step_record_id (bigint)` — FK → WorkflowStepRecord
  - `agent_uuid (UUID)` — assigned Agent
  - `status (enum: dispatched|acknowledged|timeout|completed|reassigned)`
  - `ack_deadline (timestamptz)`
  - `completed_at (timestamptz)`
- **Rules**
  - Records each dispatch attempt; history retained for audit.
  - Timeout triggers retry scheduling and marks status accordingly.

## WorkflowEvent (Audit Projection)
- **Fields**
  - `id (bigserial)`
  - `tenant_uuid (UUID)`
  - `workflow_instance_uuid (UUID)`
  - `workflow_definition_uuid (UUID)`
  - `step_id (string, nullable)`
  - `node_kind (string, nullable)`
  - `node_ref (string, nullable)`
  - `event_type (enum: state_changed|step_transition|retry_scheduled|compensation_started|compensation_completed|manual_action)`
  - `payload (jsonb)` — summary data
  - `occurred_at (timestamptz)`
- **Rules**
  - Emitted to EventBus and mirrored for ClickHouse export.

## WorkflowNodeCatalogItem
- **Fields**
  - `node_kind (string)` — adapter key, e.g. `skill.invoke`
  - `display_name_i18n_key (string)`
  - `description_i18n_key (string)`
  - `category (string)`
  - `step_type (enum: agent|system|decision|parallel|human_approval|compensation)`
  - `input_schema (jsonb)`
  - `output_schema (jsonb)`
  - `config_schema (jsonb)`
  - `required_permissions (jsonb)`
  - `required_capabilities (jsonb)`
  - `idempotency_required (bool)`
  - `compensation_supported (bool)`
- **Rules**
  - Built from registered NodeAdapters plus Skill/Capability/Knowledge/Metadata sources.
  - Builder must use this catalog; frontend mock nodes are not executable contract.

## HumanReviewTask
- **Fields**
  - `uuid (UUID)` — primary key
  - `tenant_uuid (UUID)`
  - `workflow_instance_uuid (UUID)`
  - `step_id (string)`
  - `review_type (string)`
  - `payload (jsonb)`
  - `approver_policy (jsonb)`
  - `status (enum: pending|approved|rejected|changes_requested|canceled|expired)`
  - `reviewer_user_uuid (UUID, nullable)`
  - `decision (string, nullable)`
  - `decision_payload (jsonb)`
  - `comment (text)`
  - `created_at / completed_at (timestamptz)`
- **Rules**
  - Approval must wake the waiting WorkflowInstance through Runner.
  - Rejection must not publish formal knowledge.

## WorkflowPackSeedRecord
- **Fields**
  - `uuid (UUID)` — primary key
  - `tenant_uuid (UUID)` — tenant that explicitly installed/materialized the pack
  - `workflow_key (string)`
  - `version (int)`
  - `definition_uuid (UUID)`
  - `definition_version (int)`
  - `checksum (string)`
  - `source (enum: builtin|plugin|imported)`
  - `seeded_at (timestamptz)`
- **Rules**
  - Seed must fail if required node kinds, skills, capabilities, metadata namespaces, or knowledge profiles are missing.
  - Published definition versions are immutable; seed creates a new version when checksum changes.

## WorkflowPackInstallation
- **Fields**
  - `uuid (UUID)` — stable installation record UUID
  - `tenant_uuid (UUID)` — tenant owning this installation state
  - `workflow_key (string)` — built-in or plugin-provided workflow pack key
  - `version (int)` — installed catalog version
  - `checksum (string)` — installed YAML checksum
  - `status (enum: enabled|disabled|deleted)`
  - `definition_uuid (UUID)` — current materialized WorkflowDefinition UUID when enabled
  - `definition_version (int)` — current materialized definition version
  - `source (enum: builtin|plugin|imported)`
  - `installed_at / removed_at (timestamptz)`
  - `removed_by (UUID)`
  - `last_seeded_at (timestamptz)`
  - `last_action (string)`
- **Rules**
  - Global YAML is the catalog source; regular database seed validates the catalog only.
  - A tenant WorkflowDefinition is created only after an explicit install/enable action.
  - Deleted or disabled installations must not be automatically regenerated by later seed runs.
  - Existing latest seed records are backfilled into enabled installations during migration; tenantless seed records are invalid and must be cleaned up.

## Supporting Concepts
- **RetryPolicy** (JSON schema)
  - `initial_delay_ms`, `max_retries`, `backoff_factor`, `max_delay_ms`
- **SLAConfiguration** (JSON schema)
  - `step_timeout_ms`, `overall_deadline_ms`, `breach_action`

All business references use UUIDs. Numeric auto-increment IDs may exist only as internal storage details and must not be exposed as API, event, audit, or cross-domain identifiers.
