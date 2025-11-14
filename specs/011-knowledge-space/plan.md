# Implementation Plan: Knowledge Space Provisioning & Lifecycle Governance

**Branch**: `011-docs-use-cases` | **Date**: 2025-11-12 | **Spec**: [`spec.md`](./spec.md)
**Input**: Feature specification from `/specs/011-docs-use-cases/spec.md`

**Note**: This plan captures the implementation outline prior to `/speckit.tasks`. Research, data model, and contracts generated within this command are referenced below.

## Summary

Deliver a CoreX knowledge-space service slice that (1) provisions tenant-scoped spaces with IAM/audit guardrails under 2 minutes, (2) orchestrates multimodal ingestion pipelines with ≥95% coverage & 100% masking, including deterministic dual-granularity chunking, (3) coordinates fusion strategies plus hot feedback loops, and (4) enforces 13-month read-only retention for retired assets. The plan now also covers a Web Admin provisioning wizard built on Nuxt 4 (Vue 3 + Node 20) that guides operators through tenant/department, templates, quotas, and IAM confirmation with inline validation, SLA timers, and audit previews, ensuring backend contracts are consumable through a polished UI. In addition, per `SCN-KNOWLEDGE-QA-REASON-001`, this iteration extends knowledge-space services with QA Orchestrator bridges that publish cross-space retrieval plans (≤2s), conversation-memory deltas, toolchain metadata + failover, and compliance/audit hooks so intelligent QA flows can trust every space.

## Technical Context

<!--
  ACTION REQUIRED: Replace the content in this section with the technical details
  for the project. The structure here is presented in advisory capacity to guide
  the iteration process.
-->

**Language/Version**: Go 1.24 (backend services, CLIs); Node 20 + Nuxt 4 (Vue 3 Web Admin)  
**Primary Dependencies**: Gin HTTP stack, google.golang.org/grpc (Buf toolchain), GORM, Redis, PostgreSQL, MinIO/S3 SDK, OpenTelemetry, PowerX CLI, Nuxt 4, Vue 3, Pinia, Nuxt UI, VueUse, Playwright, Vitest, Tool Runtime SDK, audit-ledger client libraries  
**Storage**: PostgreSQL (knowledge space metadata, quota, policy versions), Redis (workflow queues, throttles, conversation-memory cache), MinIO/S3 (artifact staging), pgvector driver (vector store abstraction)  
**Testing**: go test + testify for units, buf + make proto targets, contract tests under `tests/contract/knowledge_space`, integration flows under `tests/integration/knowledge_space`, Vitest unit suites for Nuxt pages/components, Playwright E2E focused on `/knowledge-spaces` wizard, QA bridge contract + failover simulations under `tests/contract/knowledge_space/qa_bridge_*`  
**Target Platform**: Linux container workloads (Kubernetes)  
**Project Type**: CoreX backend module + Nuxt 4 Web Admin console feature + QA Orchestrator bridge APIs  
**Performance Goals**: Provisioning p95 ≤120s, ingestion TTR ≤4h, fusion rollback ≤5m, feedback closure ≤24h, cross-space retrieval-plan p95 ≤2s, real-time tool success ≥99%  
**Constraints**: IAM sync success ≥99.5%, masking coverage 100%, retention 13 months read-only, dashboards update latency ≤5m, QA degrade notices must include audit IDs with 100% coverage  
**Scale/Scope**: Multi-tenant (100s of spaces, 10k docs per space), concurrent ingestion jobs (≤200 active), fusion queries up to 50 QPS admin workflows, UI concurrent editors ≤20 admins per tenant

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

1. **CoreX vs Plugin Boundary (PASS)** – Knowledge spaces live under the CoreX Knowledge domain; plan scopes work to `internal/service/...` and `pkg/corex/...` per Article 0. No plugin scaffolding or registry changes introduced.  
2. **HTTP + gRPC CRUD Rulesets (PASS)** – Plan adheres to `.specify/memory/manifest.yaml` aliases `@dev-crud-http` and `@dev-crud-grpc`: proto definitions under `api/grpc/contracts/powerx/knowledge/v1`, generated Go code in `api/grpc/gen`, HTTP handlers in `internal/transport/http/{admin,openapi}/knowledge_space`, and DTO/validation layers abide by referenced rulesets; Web Admin consumes these APIs through generated Nuxt composables without bypassing contracts.  
3. **Migration & Repository Structure (PASS)** – Data entities extend `pkg/corex/db/persistence/model/knowledge` with migrations registered via `pkg/corex/db/database/migration.go` + `cmd/database/migrate.go`, matching constitution directives for BaseRepository usage and avoiding ad-hoc data stores.  
4. **Utilities & Naming (PASS)** – Shared helpers will extend `backend/pkg/utils` where necessary; package names follow snake_case (e.g., `knowledge_space`). No deviations require complexity tracking.

## Project Structure

### Documentation (this feature)

```
specs/011-docs-use-cases/
├── plan.md              # This file (/speckit.plan output)
├── spec.md              # Approved feature spec
├── research.md          # Phase 0 findings (/speckit.plan)
├── data-model.md        # Phase 1 entity design (/speckit.plan)
├── quickstart.md        # Phase 1 onboarding (/speckit.plan)
├── contracts/           # Phase 1 API + event contracts (/speckit.plan)
└── tasks.md             # Created later by /speckit.tasks
```

### Source Code (repository root)
<!--
  ACTION REQUIRED: Replace the placeholder tree below with the concrete layout
  for this feature. Delete unused options and expand the chosen structure with
  real paths (e.g., apps/admin, packages/something). The delivered plan must
  not include Option labels.
-->

```
specs/011-docs-use-cases/
├── plan.md
├── spec.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   └── http-openapi.yaml
└── checklists/
    └── requirements.md

backend/
├── api/grpc/contracts/powerx/knowledge/v1/
├── api/grpc/gen/
├── internal/service/knowledge_space/
│   ├── qa_bridge/
│   ├── context_snapshot/
│   ├── toolchain/
│   └── compliance/
├── internal/transport/http/admin/knowledge_space/
├── internal/transport/http/openapi/knowledge_space/
│   └── qa_bridge_handlers.go
├── internal/transport/grpc/knowledge_space/
│   └── qa_bridge.go
├── internal/notifications/im/
├── pkg/corex/db/persistence/{model,repository}/knowledge/
├── pkg/corex/db/database/migration.go
├── internal/app/shared/deps.go
└── tests/{contract,integration}/knowledge_space/
    ├── qa_bridge_http_test.go
    └── qa_bridge_grpc_test.go

web-admin/
├── app/pages/knowledge-spaces/
│   ├── index.vue               # listing + create CTA
│   ├── create.vue              # multi-step wizard
│   ├── fusion.vue              # fusion strategy dashboard
│   └── feedback.vue            # feedback board
├── app/components/knowledge-spaces/
│   ├── QuotaForm.vue
│   ├── PolicySelector.vue
│   ├── IamStatusBadge.vue
│   ├── AuditPreview.vue
│   └── QaBridgeStatusCard.vue
├── app/composables/useKnowledgeSpaces.ts
├── app/stores/knowledgeSpaces.ts (Pinia)
├── app/services/knowledge-spaces/client.ts # generated SDK wrapper
├── app/services/knowledge-spaces/qaBridgeClient.ts
└── tests/e2e/knowledge-spaces.spec.ts
```

**Structure Decision**: Use the CoreX backend layout for services plus expand the existing Nuxt 4 Web Admin codebase with dedicated pages/components under `web-admin/app/pages/knowledge-spaces`. Backend contracts stay in `specs/011-docs-use-cases/` while the UI consumes the generated SDK via Nuxt composables + services (`app/services/knowledge-spaces/client.ts`), ensuring parity between UI flows and service capabilities.

## Complexity Tracking

*Fill ONLY if Constitution Check has violations that must be justified*

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| _None_ | — | — |

## Phase 0 – Research Summary

Reference: [`research.md`](./research.md)

- **Tenant-scoped naming + PostgreSQL metadata** – Composite `(tenant_id, space_name)` uniqueness with immutable `space_id` GUIDs. Resolves ambiguity around naming scope and keeps quota/audit joins transactional.
- **Observability + SLA tracking** – Standardize on OpenTelemetry → Grafana dashboards plus `reports/_state/knowledge-spaces.json` exports for leadership metrics, ensuring <5 minute detection.
- **13-month read-only retention** – Retirement transitions set artifacts to read-only, purge via scheduled jobs post 13 months, satisfying compliance.
- **Fusion degradation & rollback** – Strategy versions stored immutably; degrade to cached data on upstream failure and support <5 minute rollback via CLI automation.

All clarifications from the spec are now grounded in explicit decisions—no outstanding `NEEDS CLARIFICATION` markers remain.

## Phase 1 – Design & Contracts

### Data Model (`data-model.md`)
- Enumerates KnowledgeSpace, PolicyTemplateVersion, IngestionJob, **ArtifactBundle**, FusionStrategyVersion, FeedbackCase, IAMSyncTask, AuditTrailEntry.
- Captures identifiers, validation, and retention rules（含 ArtifactBundle 的 manifest/checksum/retention、retired state requires retention timestamp）。
- Establishes relationships used for repository interfaces and migration ordering, plus dual-granularity chunk statistics consumed by ingestion metrics.

### API Contracts (`contracts/http-openapi.yaml`)
- Defines REST endpoints for provisioning (`POST /knowledge-spaces`, `PATCH /knowledge-spaces/{id}`, retirement), ingestion jobs, fusion strategy publication, and feedback submission.
- Models quota payloads, ingestion stats, fusion weights, and SLA metadata with validation constraints (weights 0–1, severity enums, etc.).

### Quickstart (`quickstart.md`)
- Documents prerequisites (Go 1.24, buf, feature flags), proto generation commands, targeted migration runs, module bootstrap, and contract/integration test suites.
- Provides step-by-step smoke flow covering provisioning → ingestion → fusion → feedback plus observability checkpoints.

## Phase 2 – QA Orchestrator & Reasoning Alignment

- **QA Bridge Service Layer**: Implement `backend/internal/service/knowledge_space/qa_bridge` with planners that read routing configs, compute multi-space retrieval plans, encode degrade reasons, and stream telemetry (`qa.retrieval.*`). HTTP/gRPC transports expose `POST /knowledge-spaces/qa/retrieval-plan`, `POST /knowledge-spaces/qa/memory-snapshot`, `POST /knowledge-spaces/qa/tool-metadata`.
- **Conversation Memory Snapshotting**: Extend Redis/vector-backed caches under `context_snapshot` packages to persist chunk IDs + citation deltas per session, plus repository hooks to fetch last 20 rounds within 150 ms following `SCN-KNOWLEDGE-QA-CONTEXT-001`.
- **Toolchain + Failover Wiring**: Add `toolchain` subpackage that hydrates SQL/REST/rule tool configs from knowledge space metadata, validates IAM scopes, and orchestrates failover to cached outputs with audit hooks per `SCN-KNOWLEDGE-QA-TOOL-001`.
- **Compliance & Audit Hooks**: Build `compliance` helpers that wrap IAM access checks, sensitive masking, and `audit.reasoning_steps` writes for every QA call, ensuring 100% coverage demanded by `SCN-KNOWLEDGE-QA-COMPLIANCE-001`.
- **Telemetry & Dashboarding**: Publish new metrics (`qa.retrieval.latency_ms`, `qa.cross_space.hit_rate`, `qa.tool.success_rate`, `qa.feedback.loop_time`) plus degrade/alarm exports to `reports/_state/qa-reasoning.json`, and surface aggregated status in `QaBridgeStatusCard.vue`.

### Agent Context Update
- Command executed: `.specify/scripts/bash/update-agent-context.sh codex` (see terminal output in this run) to persist new technology mentions for Codex agent memory.

## Constitution Re-check

No new violations introduced after design artifacts. Proto + HTTP contracts follow declared directories, migrations stay within CoreX flow, and documentation references only CoreX modules; therefore gates remain PASS.
