# Implementation Plan: Agent Model Hub Connectivity & Governance

**Branch**: `[010-agent-model-setting]` | **Date**: 2025-11-09 | **Spec**: [`specs/010-agent-model-setting/spec.md`](./spec.md)  
**Input**: Feature specification from `/specs/010-agent-model-setting/spec.md`

## Summary

Deliver a governed Agent Model Hub that standardizes provider onboarding, adaptive routing, external connector control, and FinOps guardrails across PowerX tenants. The technical approach extends the existing Go 1.26.7 CoreX backend with dual HTTP/gRPC contracts, Buf-managed protobufs, PostgreSQL-backed registries, Redis-backed health caches, and OpenTelemetry metrics so that provider, routing, connector, and cost signals share one governance plane.

## Technical Context

**Language/Version**: Go 1.26.7 (backend services, CLIs), Node 20 (validation scripts), Go 1.21 (px-plugin CLI)
**Primary Dependencies**: Gin HTTP stack, google.golang.org/grpc, Buf toolchain, GORM + PostgreSQL, Redis, MinIO/S3 SDK, OpenTelemetry + Prometheus exporters  
**Storage**: PostgreSQL (provider profiles, routing policies, quota config), Redis (health scores, safe-mode, feature flags), MinIO/S3 (validator artifacts), Vault-backed secret store  
**Testing**: `go test ./...`, `buf lint`, `npm run lint`, scenario simulators (`scripts/ops/provider-validator.mjs`, `scripts/ops/routing-simulator.mjs`, `scripts/ops/quota-degrade.mjs`)  
**Target Platform**: Linux AMD64 containers within PowerX Core cluster  
**Project Type**: Backend/CoreX module with supporting ops scripts  
**Performance Goals**: Routing decisions ≤200 ms p95, safe-mode rollback ≤5 min, connector callback latency ≤3 s p95, cost anomaly detection ≤5 min, provider onboarding ≤24 h  
**Constraints**: Dual-transport (HTTP+gRPC) delivery, zero plaintext secrets, tenant isolation, audit log availability <1 min, component memory <256 MB  
**Scale/Scope**: Hundreds of tenants, dozens of providers/platforms, ≥10k routing decisions/min, FinOps coverage for all tenants

## Constitution Check

| Gate | Status | Notes |
|------|--------|-------|
| `HTTP_PRESENT` | ✅ Planned | REST contract captured in `contracts/http-openapi.yaml` for onboarding, routing, connectors, cost guard. |
| `GRPC_PRESENT` | ✅ Planned | gRPC schema `contracts/grpc-agent-model-hub.proto` mirrors REST capabilities with Buf compliance. |
| `PROTOBUF_DEFINED` | ✅ Planned | Buf configs reference `api/grpc/contracts/powerx/agent_model_hub/v1`; go_package_prefix aligns with `github.com/ArtisanCloud/PowerX/api/grpc/gen`. |
| `SERVER_DEFINED` | ✅ Planned | New handlers live in `internal/transport/grpc/agent_model_hub` and register via `internal/server/grpc/server.go`. |
| `MAKE_TARGETS` | ✅ Existing | Repo already exposes `proto-gen`, `proto-lint`, `proto-clean`; feature will reuse without modification. |

## Project Structure

### Documentation (this feature)

```
specs/010-agent-model-setting/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   ├── http-openapi.yaml
│   └── grpc-agent-model-hub.proto
└── tasks.md   # produced by /speckit.tasks
```

### Source Code (repository root)

```
backend/
├── config/agents/
│   ├── providers.d/
│   ├── routing.d/
│   └── feature_flags/
├── internal/server/ai/drivers/
│   ├── core/
│   ├── openai/
│   ├── google/
│   ├── hunyuan/
│   ├── jimeng/
│   ├── qwen/
│   ├── comfyui/
│   ├── stable_diffusion/
│   ├── coze/
│   └── ollama/
├── internal/server/ai/factory/
│   ├── llm/
│   └── vlm/
├── internal/service/
│   ├── provider_registry/
│   ├── model_routing/
│   ├── connector_guard/
│   └── cost_quota/
├── internal/transport/
│   ├── http/admin/agent_model_hub/
│   └── grpc/agent_model_hub/
├── pkg/corex/db/persistence/model/
├── pkg/corex/db/persistence/repository/
└── scripts/ops/
    ├── provider-validator.mjs
    ├── routing-simulator.mjs
    └── quota-degrade.mjs

api/
└── grpc/
    ├── contracts/powerx/agent_model_hub/v1/
    └── gen/go/powerx/agent_model_hub/v1/

tests/
├── integration/agent_model_hub/
├── contract/http/
└── contract/grpc/
```

**Structure Decision**: CoreX backend continues to live under `backend/internal/**` with Buf-authoritative protobufs inside `api/grpc/contracts/powerx/agent_model_hub/v1`. Ops simulators stay in `scripts/ops`, and configuration lives under `backend/config/agents/**`.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| *(none)* |  |  |

## Phase 0 – Research & Clarifications

Output: [`research.md`](./research.md)

| Topic | Decision Snapshot |
|-------|-------------------|
| Provider onboarding controls | Secrets remain Vault-managed; validation artifacts stored in MinIO with signed audit references. |
| Routing approvals & safe-mode | Approval workflow configurable per business unit; safe-mode uses Redis toggles and telemetry thresholds. |
| Connector degradation scope | Instance-level pause/resume to avoid cross-tenant impact. |
| Cost anomaly workflow | Ops must confirm recommended throttles; tenant dashboards provide read-only visibility. |

All clarifications captured in spec are now backed by rationale + alternatives in the research file; no open `NEEDS CLARIFICATION` markers remain.

## Phase 1 – Design & Contracts

Artifacts produced:
- [`data-model.md`](./data-model.md) — detailed schemas, validations, and lifecycle hooks for ProviderProfile, RoutingPolicy, ConnectorInstance, and CostQuotaLedger.
- [`contracts/http-openapi.yaml`](./contracts/http-openapi.yaml) — OpenAPI 3.1 spec for onboarding, routing, connector, and FinOps endpoints.
- [`contracts/grpc-agent-model-hub.proto`](./contracts/grpc-agent-model-hub.proto) — Buf-compliant protobuf contract for the same operations.
- [`quickstart.md`](./quickstart.md) — runbook for enabling feature flags, generating configs, executing simulators, and verifying telemetry.
- Agent context updated via `.specify/scripts/bash/update-agent-context.sh codex` to record new telemetry + governance hooks.

## Phase 2 – Implementation Outline (pre-/tasks)

Upcoming `/speckit.tasks` decomposition will align to five streams:
1. Provider onboarding pipeline (registry service, secret manager hooks, validator orchestration, audit logs).
2. Routing policy governance (schema storage, approval workflow integration, safe-mode automation, telemetry wiring).
3. Connector control plane (instance registration, OAuth/token vaulting, callback signature guard, instance-scoped degradation).
4. Cost/quota guardrails (usage ingest, anomaly detection, operator-confirm UIs/APIs, tenant dashboards).
5. Observability + runbooks (metrics exposure, alerting, dashboards, documentation updates).

Each stream inherits the dual-transport contracts and complies with Constitution gates validated above.

## Post-Design Constitution Check

| Gate | Status | Evidence |
|------|--------|----------|
| `HTTP_PRESENT` | ✅ | OpenAPI spec stored at `specs/010-agent-model-setting/contracts/http-openapi.yaml`. |
| `GRPC_PRESENT` | ✅ | Protobuf schema at `specs/010-agent-model-setting/contracts/grpc-agent-model-hub.proto`. |
| `PROTOBUF_DEFINED` | ✅ | Proto file declares `go_package` prefix mandated by constitution; Buf workflow will target `api/grpc/contracts/powerx/agent_model_hub/v1`. |
| `SERVER_DEFINED` | ✅ Plan | Handlers scoped for `internal/transport/grpc/agent_model_hub` with registration via global server (documented in Phase 2 outline). |
| `MAKE_TARGETS` | ✅ Existing | `proto-gen`, `proto-lint`, `proto-clean` remain standard make targets for regeneration. |
