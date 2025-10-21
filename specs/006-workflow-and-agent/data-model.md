# Data Model — Workflow & Agent Orchestration

## WorkflowDefinition
- **Fields**
  - `uuid (UUID)` — primary key
  - `tenant_id (UUID)` — owning tenant, required
  - `name (string)` — human readable, unique per tenant + version
  - `version (int)` — monotonic, starts at 1, composite unique with tenant/name
  - `status (enum: draft|published|archived)` — lifecycle of definition
  - `step_graph (jsonb)` — ordered/parallel topology, validated on ingest
  - `retry_policy (jsonb)` — default step retry/backoff settings
  - `compensation_policy (jsonb)` — global compensation configuration
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
  - `definition_id (UUID)` — FK → WorkflowDefinition.version
  - `tenant_id (UUID)` — copied from definition
  - `state (enum: draft|running|waiting|suspended|succeeded|failed|compensating|compensated|canceled|compensation_failed)`
  - `input_context (jsonb)` — runtime inputs/variables
  - `output_context (jsonb)` — aggregated outputs
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
  - `instance_id (UUID)` — FK → WorkflowInstance
  - `step_id (string)` — matches logical step key in definition
  - `subject_type (enum: agent|system|human)`
  - `subject_id (UUID)` — Agent UUID or operator ID when applicable
  - `grant_version (bigint)` — Tool Grant version dispatched
  - `state (enum: queued|in_progress|waiting|completed|failed|compensating|compensated)`
  - `attempt (int)` — current retry count
  - `payload_in (jsonb)` — normalized inputs sent to agent/system
  - `payload_out (jsonb)` — result payload
  - `error_reason (text)` — latest failure description
  - `started_at / finished_at (timestamptz)`
- **Relationships**
  - N:1 → WorkflowInstance
  - Optional 1:1 → `WorkflowStepCompensation`
- **Rules**
  - `(instance_id, step_id, attempt)` unique to track retries.
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
  - `agent_id (UUID)` — assigned Agent
  - `status (enum: dispatched|acknowledged|timeout|completed|reassigned)`
  - `ack_deadline (timestamptz)`
  - `completed_at (timestamptz)`
- **Rules**
  - Records each dispatch attempt; history retained for audit.
  - Timeout triggers retry scheduling and marks status accordingly.

## WorkflowEvent (Audit Projection)
- **Fields**
  - `id (bigserial)`
  - `tenant_id (UUID)`
  - `workflow_instance_id (UUID)`
  - `event_type (enum: state_changed|step_transition|retry_scheduled|compensation_started|compensation_completed|manual_action)`
  - `payload (jsonb)` — summary data
  - `occurred_at (timestamptz)`
- **Rules**
  - Emitted to EventBus and mirrored for ClickHouse export.

## Supporting Concepts
- **RetryPolicy** (JSON schema)
  - `initial_delay_ms`, `max_retries`, `backoff_factor`, `max_delay_ms`
- **SLAConfiguration** (JSON schema)
  - `step_timeout_ms`, `overall_deadline_ms`, `breach_action`

All IDs leverage existing tenant-aware UUID patterns; JSON schemas validated in service layer before persistence.
