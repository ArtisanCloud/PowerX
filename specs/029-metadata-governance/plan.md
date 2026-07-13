# Implementation Plan: Metadata Governance

**Branch**: `029-metadata-governance` | **Date**: 2026-07-12 | **Spec**: [spec.md](/private/var/www/html/ArtisanCloud/X/PowerX/Core/PowerX/specs/029-metadata-governance/spec.md)
**Input**: Feature specification from `/specs/029-metadata-governance/spec.md`

## Summary

Build Metadata Governance as a CoreX module that provides tenant-scoped dictionaries, taxonomies, tags, resource types, protected references, explicit seed flows, platform capability declarations, plugin consumption contracts, and a Nuxt admin page. The MVP does not migrate existing business modules; modules adopt metadata governance later through explicit mapping plans.

The implementation uses normal PowerX Core paths: GORM models under `backend/pkg/corex/db/persistence/model/metadata`, repositories under `backend/pkg/corex/db/persistence/repository/metadata`, services under `backend/internal/service/metadata`, admin HTTP transport under `backend/internal/transport/http/admin/metadata`, gRPC contracts under `backend/api/grpc/contracts/powerx/metadata/v1`, and Web Admin pages under `web-admin/app/pages/settings/metadata-governance`.

## Technical Context

**Language/Version**: Go 1.24 for backend/CoreX services and CLIs; TypeScript with Nuxt 4/Vue 3 for Web Admin; Buf toolchain for gRPC contracts.
**Primary Dependencies**: Gin HTTP stack, GORM, PostgreSQL JSONB, Redis only if later used for read caches, google.golang.org/grpc, Buf, Pinia/Nuxt UI, existing PowerX capability registry and IAM/RBAC services.
**Storage**: PostgreSQL tables for metadata definitions, tag bindings, references, audit events through existing audit infrastructure; seed YAML under `backend/config/metadata_governance`.
**Testing**: Go unit and integration tests via `go test`; HTTP contract tests under existing backend test layout; Nuxt/Vitest component/store tests where page logic is non-trivial; `make capability-check`; explicit migration/seed command verification.
**Target Platform**: PowerX Core backend on Linux/macOS development environments and production Linux; Web Admin in existing Nuxt app.
**Project Type**: CoreX backend + Web Admin frontend + plugin/framework contract surface.
**Performance Goals**: Admin list APIs return p95 < 200ms for normal tenant-scoped pages; dictionary/tag selector reads support indexed filters by tenant, namespace, module, resource type, status, and q; deletion conflict checks use indexed `metadata_references`/`metadata_tag_bindings` lookups rather than scanning business tables.
**Constraints**: No runtime startup AutoMigrate or AutoSeed; migrations run through explicit migrate flow only. Seed runs only through explicit command or tenant bootstrap hook. All business object tables have UUIDs; relationship tables may omit UUID but must reference object UUIDs. User-visible text must use i18n resources; no UUID/code as primary label. No fallback to plugin private metadata in delegated mode.
**Scale/Scope**: MVP covers governance center, APIs, seed, capability declarations, plugin read/replace-binding contract, and admin page. Existing customer/knowledge/agent/media business modules are not migrated in this feature.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Gate | Status | Notes |
|------|--------|-------|
| COREX_DECLARED | PASS | Metadata governance is a CoreX module: `corex.metadata`. It is not a plugin and must not use `plugins/registry.json`. |
| NO_PLUGIN_REGISTRY | PASS | Plugin work is framework consumption contract only; no plugin lifecycle registration is introduced. |
| COREX_LAYOUT_MATCH | PASS | Planned paths match `internal/service`, `internal/transport/http`, `internal/transport/grpc`, `pkg/corex/db/persistence/model`, and repository layout. |
| COREX_DUAL_TRANSPORT | PASS | REST/OpenAPI and gRPC design contracts are generated in `contracts/`; implementation must create both transports unless a later spec explicitly narrows the gRPC surface. |
| COREX_BUF_CONFIG | PASS | gRPC implementation must use existing `backend/api/grpc/contracts/{buf.yaml,buf.gen.yaml}` and output to `backend/api/grpc/gen/go`. |
| COREX_SERVER_WIRING | PASS | HTTP wiring goes through existing admin router; gRPC wiring goes through global server/bootstrap, not module-owned `grpc.NewServer`. |
| COREX_MIGRATION_WIRING | PASS | Models are registered in centralized CoreX migration flow and invoked only by explicit migrate command, not backend startup. |
| HTTP_PRESENT | PASS | REST contract and admin routes are planned under `/api/v1/admin/metadata/...`. |
| GRPC_PRESENT | PASS | gRPC contract is planned as `powerx.metadata.v1.MetadataGovernanceService`. |
| PROTOBUF_DEFINED | PASS | Design proto exists in `contracts/metadata.proto`; implementation must copy/adapt into authoritative `backend/api/grpc/contracts/powerx/metadata/v1/metadata.proto`. |
| SERVER_DEFINED | PASS | Plan requires global gRPC bootstrap registration with auth/tenant/logging/recovery interceptors. |
| MAKE_TARGETS | PASS | Plan requires `proto-gen`, `proto-lint`, `proto-clean`, `migrate`, metadata seed command, and `capability-check`. |
| SECURITY_RBAC_AUDIT | PASS | Read/manage permissions, capability mappings, tenant isolation, and audit events are required. |
| OBSERVABILITY | PASS | Service logs include `trace_id`, `tenant_uuid`, metadata object UUIDs, operation, and failure codes. |

## Project Structure

### Documentation (this feature)

```text
specs/029-metadata-governance/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   ├── http-openapi.yaml
│   └── metadata.proto
└── tasks.md
```

### Source Code (repository root)

```text
backend/
├── api/grpc/contracts/powerx/metadata/v1/
│   └── metadata.proto
├── api/grpc/gen/go/powerx/metadata/v1/
├── cmd/
│   └── metadata_seed/
├── config/
│   ├── metadata_governance/
│   │   └── seed.yaml
│   └── platform_capabilities/
│       └── metadata.yaml
├── internal/
│   ├── dto/metadata/
│   ├── service/metadata/
│   ├── transport/grpc/metadata/
│   └── transport/http/admin/metadata/
├── pkg/corex/db/
│   ├── database/migration.go
│   └── persistence/
│       ├── model/metadata/
│       └── repository/metadata/
└── internal/tests/
    ├── http/admin/metadata/
    └── integration/metadata/

web-admin/
├── app/pages/settings/metadata-governance/
│   └── index.vue
├── app/components/settings/metadata-governance/
├── app/composables/api/metadata-governance.ts
├── app/stores/metadata-governance.ts
├── app/types/metadata-governance.ts
└── i18n/locales/

docs/guides/develop/
└── metadata_governance.md
```

**Structure Decision**: Use CoreX module structure. Metadata governance is a platform domain and must be implemented in PowerX Core, not as a plugin. Plugin framework work is limited to consuming the governed contract and must not introduce plugin-managed metadata definitions in MVP.

## Phase 0: Research

Phase 0 output: [research.md](/private/var/www/html/ArtisanCloud/X/PowerX/Core/PowerX/specs/029-metadata-governance/research.md)

Research decisions:

- Metadata definitions are tenant-scoped rows, not global shared rows at runtime.
- i18n values are JSONB maps with required `zh-CN`; admin views may show `zh-CN` with an explicit missing-locale marker when requested locale is absent.
- Tag binding writes require an enabled resource type validator.
- Protected reference registration is part of the consistency boundary for adopting modules.
- Seed is explicit command or tenant bootstrap only; backend startup must not seed.
- Capability publication uses business authorization units, not one raw route per endpoint.

## Phase 1: Design & Contracts

Phase 1 outputs:

- [data-model.md](/private/var/www/html/ArtisanCloud/X/PowerX/Core/PowerX/specs/029-metadata-governance/data-model.md)
- [contracts/http-openapi.yaml](/private/var/www/html/ArtisanCloud/X/PowerX/Core/PowerX/specs/029-metadata-governance/contracts/http-openapi.yaml)
- [contracts/metadata.proto](/private/var/www/html/ArtisanCloud/X/PowerX/Core/PowerX/specs/029-metadata-governance/contracts/metadata.proto)
- [quickstart.md](/private/var/www/html/ArtisanCloud/X/PowerX/Core/PowerX/specs/029-metadata-governance/quickstart.md)

Design requirements:

- REST is the management and plugin-invocation binding contract.
- gRPC provides CoreX service parity for internal/Core consumers and generated clients.
- Admin REST paths use `/api/v1/admin/metadata/...` and user JWT/RBAC.
- Plugin service calls use `/api/v1/tenant/invocations` with metadata capabilities or approved service bindings, not plugin-private fallback lists.
- Capability declarations must be added to `backend/config/platform_capabilities/metadata.yaml` and verified by `make capability-check`.

## Post-Design Constitution Check

| Gate | Status | Evidence |
|------|--------|----------|
| No unresolved clarifications | PASS | Clarifications section in `spec.md` contains 5 accepted decisions. |
| REST + gRPC contracts present | PASS | `contracts/http-openapi.yaml` and `contracts/metadata.proto` generated. |
| CoreX path compliance | PASS | Plan maps backend to CoreX directories and migration flow. |
| No runtime migration/seed | PASS | Plan and quickstart require explicit migrate and seed commands. |
| Capability governance | PASS | Plan requires `metadata.yaml`, `make capability-check`, and no direct validator-only bypass. |
| i18n compliance | PASS | Data model and contracts require i18n maps and missing-locale marker. |
| UUID relationship compliance | PASS | Data model uses object UUID references for all external relationships. |

## Complexity Tracking

No constitutional violations are introduced. The module is broad, but it is a single CoreX governance domain with four sub-areas, not four independent systems. The chosen structure avoids generic polymorphic metadata tables for definitions while still using explicit resource type registration for tag bindings and protected references.
