# Tasks: Metadata Governance

**Input**: Design documents from `/specs/029-metadata-governance/`
**Prerequisites**: `plan.md`, `spec.md`, `research.md`, `data-model.md`, `contracts/`, `quickstart.md`

**Tests**: Included because `spec.md`, `plan.md`, and `quickstart.md` explicitly require service/repository/HTTP/Web Admin verification.

**Organization**: Tasks are grouped by user story so each story can be implemented and tested independently after the foundational phase.

## Format: `[ID] [P] [Story] Description`

- **[P]**: Can run in parallel because it touches different files and has no dependency on another task in the same phase.
- **[Story]**: User story label from `spec.md`.
- Every task names concrete files or directories.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Create the CoreX metadata-governance skeleton, contracts, and configuration locations.

- [ ] T001 Create backend metadata package directories: `backend/pkg/corex/db/persistence/model/metadata/`, `backend/pkg/corex/db/persistence/repository/metadata/`, `backend/internal/dto/metadata/`, `backend/internal/service/metadata/`, `backend/internal/transport/http/admin/metadata/`, `backend/internal/transport/grpc/metadata/`.
- [ ] T002 Create Web Admin metadata-governance directories: `web-admin/app/pages/settings/metadata-governance/`, `web-admin/app/components/settings/metadata-governance/`, `web-admin/app/composables/api/`, `web-admin/app/stores/`, `web-admin/app/types/metadata-governance.ts`.
- [ ] T003 [P] Copy/adapt the design proto from `specs/029-metadata-governance/contracts/metadata.proto` to authoritative `backend/api/grpc/contracts/powerx/metadata/v1/metadata.proto`.
- [ ] T004 [P] Add canonical seed directory and placeholder schema file in `backend/config/metadata_governance/seed.schema.yaml` and initial seed file in `backend/config/metadata_governance/seed.yaml`.
- [ ] T005 [P] Add metadata capability declaration file `backend/config/platform_capabilities/metadata.yaml` with the eight capability IDs and permission codes from `data-model.md`.
- [ ] T006 [P] Add developer guide stub `docs/guides/develop/metadata_governance.md` linking to `specs/029-metadata-governance/{spec.md,plan.md,data-model.md,quickstart.md}`.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core infrastructure required before any user story can work.

**CRITICAL**: No user story implementation should start until this phase is complete.

### Tests First

- [ ] T007 [P] Add repository migration/model test skeleton in `backend/pkg/corex/db/persistence/model/metadata/metadata_models_test.go` covering UUID fields, JSONB i18n fields, and unique index expectations.
- [ ] T008 [P] Add service validation test skeleton in `backend/internal/service/metadata/validation_test.go` covering invalid namespace/code/resource_type, missing `zh-CN`, and forbidden UUID-as-code cases.
- [ ] T009 [P] Add HTTP route contract test skeleton in `backend/internal/tests/http/admin/metadata/routes_test.go` asserting `/api/v1/admin/metadata/*` routes exist and require authenticated tenant context.

### Implementation

- [ ] T010 [P] Implement shared metadata model constants and status enums in `backend/pkg/corex/db/persistence/model/metadata/types.go`.
- [ ] T011 Implement GORM models for dictionary namespace/item, taxonomy/node, tag/binding, resource type, and reference in `backend/pkg/corex/db/persistence/model/metadata/models.go`.
- [ ] T012 Register metadata models in centralized migration flow in `backend/pkg/corex/db/database/migration.go`; do not add startup AutoMigrate.
- [ ] T013 [P] Implement shared DTO types for i18n maps, display fields, pagination filters, status, and reference summaries in `backend/internal/dto/metadata/common.go`.
- [ ] T014 [P] Implement metadata validation helpers in `backend/internal/service/metadata/validation.go` for machine identifiers, required locale, status transitions, and UUID rejection.
- [ ] T015 [P] Implement repository base and query filter structs in `backend/pkg/corex/db/persistence/repository/metadata/repository.go`.
- [ ] T016 Implement service constructor and dependency struct in `backend/internal/service/metadata/service.go`, depending on DB, transaction helper, audit publisher, permission checker, and resource validator registry.
- [ ] T017 [P] Implement resource validator registry interface in `backend/internal/service/metadata/resource_validator.go`.
- [ ] T018 [P] Implement shared error codes in `backend/internal/service/metadata/errors.go` and HTTP error mapping in `backend/internal/transport/http/admin/metadata/error_mapping.go`.
- [ ] T019 Implement admin HTTP router skeleton in `backend/internal/transport/http/admin/metadata/api.go` and mount it from the existing admin router entrypoint.
- [ ] T020 Implement gRPC service skeleton in `backend/internal/transport/grpc/metadata/server.go` and wire it through the global gRPC bootstrap without creating a module-owned gRPC server.
- [ ] T020a Add contract parity checks ensuring every REST PATCH/update operation has a matching gRPC Update RPC and gRPC update scalar fields preserve explicit presence for false, zero, disabled, archived, and empty values.
- [ ] T021 Add metadata permissions to IAM seed/config path used for platform permissions, including `metadata.dictionary:read/manage`, `metadata.taxonomy:read/manage`, `metadata.tag:read/manage`, and `metadata.resource_type:read/manage`.
- [ ] T022 Add platform capability metadata in `backend/config/platform_capabilities/metadata.yaml` with `permission_code`, `agent_usable: false`, `risk_level`, `actor_context`, `resource_scope`, and no default `sts_direct: true` for admin-user bindings.
- [ ] T023 Add explicit seed command entrypoint `backend/cmd/metadata_seed/main.go` that validates seed files and requires explicit tenant/bootstrap input; fail fast when required seed definitions are missing.
- [ ] T024 Add Make targets for explicit seed and validation in `make_files/metadata.mk` and include it from the root `Makefile`; do not alias seed to backend startup.

**Checkpoint**: Foundation ready. Models migrate only through explicit migration, seed is explicit, routes/contracts are mounted, and story work can proceed.

---

## Phase 3: User Story 1 - 统一管理数据字典 (Priority: P1) MVP

**Goal**: Tenant administrators can create dictionary namespaces/items, list/filter them, disable items, preserve historical readability, and block deletion when references exist.

**Independent Test**: Create one namespace with five items, disable one item, confirm it is unavailable for new selection but still readable, and verify referenced item deletion returns conflict with reference summary.

### Tests for User Story 1

- [ ] T025 [P] [US1] Add repository tests for dictionary namespace/item uniqueness, tenant isolation, status filters, and sort order in `backend/pkg/corex/db/persistence/repository/metadata/dictionary_repository_test.go`.
- [ ] T026 [P] [US1] Add service tests for dictionary create/update/list/disable/delete conflict in `backend/internal/service/metadata/dictionary_service_test.go`.
- [ ] T027 [P] [US1] Add HTTP contract tests for dictionary endpoints in `backend/internal/tests/http/admin/metadata/dictionary_http_test.go`.

### Implementation for User Story 1

- [ ] T028 [P] [US1] Implement dictionary DTOs in `backend/internal/dto/metadata/dictionary.go` matching `contracts/http-openapi.yaml`.
- [ ] T029 [US1] Implement dictionary repository methods in `backend/pkg/corex/db/persistence/repository/metadata/dictionary_repository.go`.
- [ ] T030 [US1] Implement dictionary service methods in `backend/internal/service/metadata/dictionary_service.go` including immutable namespace/code validation and protected-reference delete checks.
- [ ] T031 [US1] Implement dictionary HTTP handlers in `backend/internal/transport/http/admin/metadata/dictionary_handler.go` using unified PowerX response helpers.
- [ ] T032 [US1] Implement dictionary gRPC methods in `backend/internal/transport/grpc/metadata/dictionary_server.go`.
- [ ] T033 [US1] Add dictionary capability bindings to `backend/config/platform_capabilities/metadata.yaml` for list/create/update/delete REST routes under `com.corex.metadata.dictionary.read/manage`.
- [ ] T034 [US1] Add dictionary seed parsing/upsert support in `backend/internal/service/metadata/seed_dictionary.go`.

**Checkpoint**: US1 works independently through service, REST, gRPC, and explicit seed.

---

## Phase 4: User Story 2 - 维护可控分类体系 (Priority: P1)

**Goal**: Tenant administrators can manage taxonomies and taxonomy nodes, enforce max depth, move nodes safely, and block circular or referenced deletes.

**Independent Test**: Create a taxonomy with at least three levels, move a node successfully, verify circular move and max-depth violation fail, and verify referenced node deletion returns conflict.

### Tests for User Story 2

- [ ] T035 [P] [US2] Add repository tests for taxonomy/node tree queries, path/depth persistence, and tenant isolation in `backend/pkg/corex/db/persistence/repository/metadata/taxonomy_repository_test.go`.
- [ ] T036 [P] [US2] Add service tests for taxonomy node create/move/max-depth/circular/concurrency/delete-conflict in `backend/internal/service/metadata/taxonomy_service_test.go`.
- [ ] T037 [P] [US2] Add HTTP contract tests for taxonomy endpoints in `backend/internal/tests/http/admin/metadata/taxonomy_http_test.go`.

### Implementation for User Story 2

- [ ] T038 [P] [US2] Implement taxonomy DTOs in `backend/internal/dto/metadata/taxonomy.go` matching `contracts/http-openapi.yaml`.
- [ ] T039 [US2] Implement taxonomy repository methods in `backend/pkg/corex/db/persistence/repository/metadata/taxonomy_repository.go`.
- [ ] T040 [US2] Implement taxonomy service methods in `backend/internal/service/metadata/taxonomy_service.go`, including path/depth recalculation and optimistic concurrency validation.
- [ ] T041 [US2] Implement taxonomy HTTP handlers in `backend/internal/transport/http/admin/metadata/taxonomy_handler.go`.
- [ ] T042 [US2] Implement taxonomy gRPC methods in `backend/internal/transport/grpc/metadata/taxonomy_server.go`.
- [ ] T043 [US2] Add taxonomy capability bindings to `backend/config/platform_capabilities/metadata.yaml` for read/manage REST routes.
- [ ] T044 [US2] Add taxonomy seed parsing/upsert support in `backend/internal/service/metadata/seed_taxonomy.go`.

**Checkpoint**: US2 works independently and does not require business module migration.

---

## Phase 5: User Story 3 - 治理标签和标签绑定 (Priority: P1)

**Goal**: Tenant administrators can create tags per resource type, replace bindings, view usage, merge tags, audit changes, and block deletion of bound tags.

**Independent Test**: Register a bindable resource type, create tags, bind tags to a resource, merge two tags, verify audit event, and verify bound tag deletion is rejected.

### Tests for User Story 3

- [ ] T045 [P] [US3] Add repository tests for tag uniqueness, binding replace, usage counts, and delete conflict in `backend/pkg/corex/db/persistence/repository/metadata/tag_repository_test.go`.
- [ ] T046 [P] [US3] Add service tests for tag create/update/merge/delete and binding replace with enabled-only tags in `backend/internal/service/metadata/tag_service_test.go`.
- [ ] T047 [P] [US3] Add HTTP contract tests for tag and tag-binding endpoints in `backend/internal/tests/http/admin/metadata/tag_http_test.go`.

### Implementation for User Story 3

- [ ] T048 [P] [US3] Implement tag DTOs in `backend/internal/dto/metadata/tag.go` and tag binding DTOs in `backend/internal/dto/metadata/tag_binding.go`.
- [ ] T049 [US3] Implement tag repository methods in `backend/pkg/corex/db/persistence/repository/metadata/tag_repository.go`.
- [ ] T050 [US3] Implement tag binding repository methods in `backend/pkg/corex/db/persistence/repository/metadata/tag_binding_repository.go`.
- [ ] T051 [US3] Implement tag service methods in `backend/internal/service/metadata/tag_service.go`, including merge, audit hooks, delete protection, and enabled-only binding validation.
- [ ] T052 [US3] Implement tag binding service methods in `backend/internal/service/metadata/tag_binding_service.go`, including transactional replace.
- [ ] T053 [US3] Implement tag HTTP handlers in `backend/internal/transport/http/admin/metadata/tag_handler.go` and tag binding handlers in `backend/internal/transport/http/admin/metadata/tag_binding_handler.go`.
- [ ] T054 [US3] Implement tag gRPC methods in `backend/internal/transport/grpc/metadata/tag_server.go`.
- [ ] T055 [US3] Add tag capability bindings to `backend/config/platform_capabilities/metadata.yaml` for read/manage REST routes.
- [ ] T056 [US3] Add tag seed parsing/upsert support in `backend/internal/service/metadata/seed_tag.go`.

**Checkpoint**: US3 works independently when a resource validator is registered.

---

## Phase 6: User Story 4 - 管理资源类型和引用完整性 (Priority: P2)

**Goal**: Platform or tenant administrators can register resource types and the system can validate resource existence, tenant boundary, and protected references.

**Independent Test**: Register a resource type without validator and verify tag binding write fails; register one with validator and verify binding succeeds; verify metadata reference registration rollback behavior.

### Tests for User Story 4

- [ ] T057 [P] [US4] Add repository tests for resource type and metadata reference uniqueness/index behavior in `backend/pkg/corex/db/persistence/repository/metadata/resource_reference_repository_test.go`.
- [ ] T058 [P] [US4] Add service tests for resource type registration, missing validator write rejection, enabled validator success, and reference rollback in `backend/internal/service/metadata/resource_reference_service_test.go`.
- [ ] T059 [P] [US4] Add HTTP contract tests for resource type endpoints in `backend/internal/tests/http/admin/metadata/resource_type_http_test.go`.

### Implementation for User Story 4

- [ ] T060 [P] [US4] Implement resource type DTOs in `backend/internal/dto/metadata/resource_type.go` and reference DTOs in `backend/internal/dto/metadata/reference.go`.
- [ ] T061 [US4] Implement resource type repository in `backend/pkg/corex/db/persistence/repository/metadata/resource_type_repository.go`.
- [ ] T062 [US4] Implement metadata reference repository in `backend/pkg/corex/db/persistence/repository/metadata/reference_repository.go`.
- [ ] T063 [US4] Implement resource type service in `backend/internal/service/metadata/resource_type_service.go`, including validator status resolution and immutable `resource_type` enforcement.
- [ ] T064 [US4] Implement metadata reference service in `backend/internal/service/metadata/reference_service.go` with transactional register/replace/delete helpers for adopting modules.
- [ ] T065 [US4] Integrate `ResourceValidatorRegistry` into `backend/internal/service/metadata/tag_binding_service.go` so writes fail when validator is missing or disabled.
- [ ] T066 [US4] Implement resource type HTTP handlers in `backend/internal/transport/http/admin/metadata/resource_type_handler.go`.
- [ ] T067 [US4] Implement resource type gRPC methods in `backend/internal/transport/grpc/metadata/resource_type_server.go`.
- [ ] T068 [US4] Add resource type capability bindings to `backend/config/platform_capabilities/metadata.yaml`.

**Checkpoint**: US4 provides the integrity layer required by plugin/framework tag binding writes.

---

## Phase 7: User Story 5 - 插件消费底座元数据 (Priority: P2)

**Goal**: Plugins can consume governed metadata through framework/client contracts in delegated mode and fail clearly in local mode when canonical seed is missing.

**Independent Test**: A plugin-style client reads dictionary items, taxonomy nodes, tags, resolves resource type, and replaces tag bindings through governed capability paths; missing capability or missing local seed fails explicitly.

### Tests for User Story 5

- [ ] T069 [P] [US5] Add delegated metadata client tests in `backend/internal/tests/integration/metadata/plugin_metadata_client_test.go` covering read and replace-binding capability paths.
- [ ] T070 [P] [US5] Add seed failure tests in `backend/internal/service/metadata/seed_service_test.go` for missing canonical definitions and invalid schema.
- [ ] T071 [P] [US5] Add capability registry verification test in `backend/internal/tests/integration/metadata/metadata_capability_test.go` for missing permission and successful tenant registration.

### Implementation for User Story 5

- [ ] T072 [US5] Implement metadata seed loader and schema validator in `backend/internal/service/metadata/seed_loader.go`.
- [ ] T073 [US5] Implement seed service orchestration in `backend/internal/service/metadata/seed_service.go` for explicit command and tenant bootstrap hook entrypoints.
- [ ] T074 [US5] Wire `backend/cmd/metadata_seed/main.go` to `seed_service.go` with required tenant UUID and dry-run flags.
- [ ] T075 [US5] Add tenant bootstrap integration point for metadata seed in the tenant provisioning service path, ensuring failures abort bootstrap and do not run at backend startup.
- [ ] T076 [US5] Add metadata capability entries to capability required/audit config if needed in `backend/config/capability_audit_required.yaml`.
- [ ] T077 [US5] Run `make capability-gen` and adjust `backend/config/platform_capabilities/metadata.yaml` or audit ignore entries so `make capability-check` passes without opening unmanaged admin STS direct paths.
- [ ] T078 [US5] Document plugin framework metadata client contract in `docs/guides/develop/plugin_agent_skill_bridge.md` and `docs/guides/develop/metadata_governance.md`, including delegated/local behavior and no private fallback rule.

**Checkpoint**: US5 is consumable by plugin teams without requiring business module migration.

---

## Phase 8: User Story 6 - 元数据治理管理页面可用 (Priority: P2)

**Goal**: Admin users can manage dictionaries, taxonomies, tags, and resource types in one settings page with clear loading, no-permission, missing-selection, empty, and error states.

**Independent Test**: Open `设置 > 元数据治理`, operate each tab, verify filters, permissions, i18n labels, missing locale markers, and differentiated empty/error states.

### Tests for User Story 6

- [ ] T079 [P] [US6] Add store/composable tests for metadata API state handling in `web-admin/app/stores/metadata-governance.test.ts`.
- [ ] T080 [P] [US6] Add page component tests for tab empty/no-permission/loading/error/missing-selection states in `web-admin/app/pages/settings/metadata-governance/index.test.ts`.

### Implementation for User Story 6

- [ ] T081 [P] [US6] Implement Web Admin TypeScript types in `web-admin/app/types/metadata-governance.ts` matching `contracts/http-openapi.yaml`.
- [ ] T082 [P] [US6] Implement API composable in `web-admin/app/composables/api/metadata-governance.ts` without hard-coded user-visible copy.
- [ ] T083 [US6] Implement Pinia store in `web-admin/app/stores/metadata-governance.ts` with separate states for loading, no permission, missing selection, empty data, and backend error.
- [ ] T084 [P] [US6] Add reusable state and filter components under `web-admin/app/components/settings/metadata-governance/`.
- [ ] T085 [US6] Implement settings page `web-admin/app/pages/settings/metadata-governance/index.vue` with tabs for dictionaries, taxonomies, tags, and resource types.
- [ ] T086 [US6] Add settings menu/route entry for metadata governance using existing settings navigation files.
- [ ] T087 [US6] Add locale keys for all page labels, buttons, validation messages, empty states, toasts, and confirmations in `web-admin/i18n/locales/zh-CN.json` and supported locale files.
- [ ] T088 [US6] Add UI permission checks so read permissions show tabs and manage permissions control create/edit/delete/merge actions.
- [ ] T089 [US6] Add missing-locale visual marker support in page rows and forms when API returns `display_locale_missing=true`.

**Checkpoint**: US6 provides the operational UI for the completed backend metadata governance surface.

---

## Final Phase: Polish & Cross-Cutting Concerns

**Purpose**: Validation, documentation, observability, and release readiness across all stories.

- [ ] T090 [P] Add structured logs for metadata operations in `backend/internal/service/metadata/*.go` with `trace_id`, `tenant_uuid`, operation, object UUID, and stable error code.
- [ ] T091 [P] Add audit event integration for create/update/disable/archive/delete/merge/binding changes in `backend/internal/service/metadata/audit.go`.
- [ ] T092 [P] Update `docs/guides/develop/open_capability/readme.md` with metadata capability discovery and invocation examples.
- [ ] T093 [P] Update `docs/plan/metadata-governance/{README.md,mechanisms.md,pages.md,rules.md}` if implementation decisions differ from the generated spec artifacts.
- [ ] T094 Run `make proto-gen` and `make proto-lint`; commit generated files under `backend/api/grpc/gen/go/powerx/metadata/v1/`.
- [ ] T095 Run explicit migration verification using the repository's migration command; confirm no backend startup AutoMigrate/AutoSeed was added.
- [ ] T096 Run backend tests: `cd backend && go test ./internal/service/metadata/... ./pkg/corex/db/persistence/repository/metadata/... ./internal/tests/http/admin/metadata/... ./internal/tests/integration/metadata/...`.
- [ ] T097 Run `make capability-check` and fix capability declarations, generated raw capabilities, or audit ignore entries until it passes.
- [ ] T098 Run Web Admin tests: `cd web-admin && npm run test`.
- [ ] T099 Run manual quickstart smoke path from `specs/029-metadata-governance/quickstart.md` and record any deviations in `docs/guides/develop/metadata_governance.md`.
- [ ] T100 Review all new user-visible text for i18n compliance and all API/DTO/event references for UUID-only business object references.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 Setup**: No dependencies.
- **Phase 2 Foundational**: Depends on Phase 1. Blocks every user story.
- **US1 Dictionaries**: Depends on Phase 2.
- **US2 Taxonomies**: Depends on Phase 2.
- **US3 Tags/Bindings**: Depends on Phase 2 and uses the resource validator abstraction from T017; full binding success path depends on US4 resource type implementation.
- **US4 Resource Types/References**: Depends on Phase 2; unlocks strict tag binding writes.
- **US5 Plugin Consumption/Seed**: Depends on US1, US2, US3, and US4 service surfaces plus capability declarations.
- **US6 Admin Page**: Depends on REST contracts and at least one completed backend story; full page acceptance depends on US1-US4.
- **Final Phase**: Depends on the desired user stories being complete.

### User Story Dependencies

- **US1 (P1)**: First MVP increment; no dependency on other user stories.
- **US2 (P1)**: Can run after foundation; independent from US1 except shared metadata infrastructure.
- **US3 (P1)**: Can create/manage tags after foundation; successful binding write requires US4 validator enforcement to be complete.
- **US4 (P2)**: Integrity layer for polymorphic resource references; can run in parallel with US1/US2/US3 after foundation.
- **US5 (P2)**: Requires backend read/write surfaces and capability declarations.
- **US6 (P2)**: Can start API types/composables after contracts, but full UI validation requires backend endpoints.

### Within Each User Story

- Write tests first and verify they fail.
- Implement DTOs and repository methods before service methods.
- Implement services before HTTP/gRPC handlers.
- Update capability bindings after routes are known.
- Validate each story at its checkpoint before proceeding.

---

## Parallel Execution Examples

### Foundation

```text
Task: T007 model tests
Task: T008 validation tests
Task: T009 route tests
Task: T013 common DTOs
Task: T014 validation helpers
Task: T017 resource validator registry
```

### US1 Dictionaries

```text
Task: T025 dictionary repository tests
Task: T026 dictionary service tests
Task: T027 dictionary HTTP tests
Task: T028 dictionary DTOs
```

### US2 Taxonomies

```text
Task: T035 taxonomy repository tests
Task: T036 taxonomy service tests
Task: T037 taxonomy HTTP tests
Task: T038 taxonomy DTOs
```

### US3 Tags

```text
Task: T045 tag repository tests
Task: T046 tag service tests
Task: T047 tag HTTP tests
Task: T048 tag DTOs
```

### US6 Web Admin

```text
Task: T079 store/composable tests
Task: T080 page state tests
Task: T081 TypeScript types
Task: T082 API composable
Task: T084 reusable components
```

---

## Implementation Strategy

### MVP First

1. Complete Phase 1 and Phase 2.
2. Complete US1 dictionaries.
3. Validate US1 through service, REST, gRPC, seed, and manual quickstart dictionary flow.
4. Stop and demo the first usable metadata governance slice.

### Incremental Delivery

1. Add US2 taxonomy management.
2. Add US3 tag management.
3. Add US4 resource type and reference integrity.
4. Add US5 plugin/framework consumption and explicit seed hardening.
5. Add US6 full admin page or begin its API/types work in parallel once contracts stabilize.

### Strict Rules During Implementation

- Do not add backend startup AutoMigrate or AutoSeed.
- Do not add compatibility fallbacks for plugin private metadata in delegated mode.
- Do not use numeric IDs in API, event, audit, or relationship DTOs for business object references.
- Do not show UUID/code as the primary human-facing label.
- Do not introduce user-visible text outside locale resources.
- Do not bypass capability governance by only editing STS/auth validators.
