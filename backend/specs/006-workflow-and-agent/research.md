# Phase 0 Research — Workflow & Agent Orchestration

## Decision: Workflow state persistence on PostgreSQL + GORM
- **Rationale**: CoreX already uses PostgreSQL with GORM for transactional domains, providing migrations, tenant-aware schemas, and HA. Keeping workflow definitions/instances in the same cluster guarantees transactional integrity for retries/compensation metadata.
- **Alternatives considered**:
  - Dedicated workflow engine (e.g., Temporal) — rejected due to operational overhead and divergence from CoreX stack.
  - Document store (e.g., MongoDB) — rejected because relations (definition ↔ instances ↔ steps) benefit from SQL constraints.

## Decision: Redis-backed step dispatch queue
- **Rationale**: Existing CoreX infrastructure provisioned Redis for scheduling/Event Fabric; using Redis sorted sets/streams enables reliable delayed retries and SLA timers without introducing new infrastructure.
- **Alternatives considered**:
  - PostgreSQL advisory locks — insufficient for high-frequency retry scheduling.
  - Kafka-based scheduler — overkill for initial stage, adds end-to-end latency.

## Decision: Agent communication via gRPC + Tool Grant validation
- **Rationale**: Agents already integrate through gRPC with shared interceptors; keeping Agent orchestration on gRPC ensures consistent authN/z (Tool Grants) and streaming support for long-running steps.
- **Alternatives considered**:
  - REST callbacks — incompatible with streaming and adds extra auth surface.
  - Direct message bus commands — bypasses existing Grant enforcement.

## Decision: SLA monitoring and observability via existing OTEL stack
- **Rationale**: CoreX services export metrics/traces through OTEL; instrumenting workflow scheduler with latency/histogram metrics fits existing dashboards and alerting (no new tooling).
- **Alternatives considered**:
  - Custom monitoring service — redundant.
  - Relying solely on logs — insufficient for proactive SLA alerts.

All open clarifications resolved; proceed to design.
