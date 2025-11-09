# Research Notes – Agent Model Hub Connectivity & Governance

## Decision 1: Vault + MinIO pipeline for provider onboarding artifacts
- **Rationale**: Keeps secrets centralized in Vault while validation logs/artifacts (latency traces, sandbox transcripts) are stored in MinIO with signed audit links, matching existing ops tooling.
- **Alternatives considered**:
  - *Direct PostgreSQL storage*: rejected due to size and binary payload concerns.
  - *External artifact service*: would add latency and duplicate IAM integrations.

## Decision 2: Configurable routing approval workflow
- **Rationale**: Different business units have distinct change-control policies; allowing each BU to define approvers while still logging outcomes lets us satisfy governance without bottlenecking releases.
- **Alternatives considered**:
  - *Universal two-person approval*: simpler but conflicts with teams that already run automated policy pipelines.
  - *No approval*: violates constitution-linked audit expectations.

## Decision 3: Instance-level connector degradation
- **Rationale**: Pausing only the failing Coze/n8n instance prevents healthy tenants from losing automation while still protecting the platform when callbacks fail >5%.
- **Alternatives considered**:
  - *Global pause*: safer but causes avoidable outages for unaffected tenants.
  - *Retry-only strategy*: prolongs incident impact and hides systemic issues.

## Decision 4: Operator-confirmed cost enforcement + tenant dashboards
- **Rationale**: Keeping humans-in-the-loop for throttle/degrade actions avoids unsafe automation, yet tenants still gain transparency via read-only dashboards to self-serve investigations.
- **Alternatives considered**:
  - *Fully automatic throttling*: faster but high false-positive risk during telemetry glitches.
  - *Alert-only*: delays enforcement, increasing budget exposure.

## Decision 5: Dual-transport contract inheritance via Buf
- **Rationale**: Using Buf-managed protobufs that mirror OpenAPI resources guarantees parity between HTTP and gRPC transports and satisfies constitution gates without duplicating logic.
- **Alternatives considered**:
  - *HTTP-only*: violates Article X dual-transport mandate.
  - *Manual proto definitions outside Buf*: drifts from repo standards and would fail CI.
