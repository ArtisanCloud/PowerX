# Tasks: Customer Contact

## Phase 1 — Core model and service

- [x] T001 Add `Contact` and `ContactIdentity` models, UUID references, table constants, indexes, and controlled enums.
- [x] T002 Register Contact models through `migrateCustomerModels` in the central Core migration entry.
- [x] T003 Add tenant/Customer-scoped Contact repositories.
- [x] T004 Implement Contact service validation, transactions, audit, and stable errors.
- [x] T005 Add focused model, repository, and service tests for triple ownership, role/tag validation, and identity uniqueness.

## Phase 2 — Core contracts and governance

- [x] T006 Add Admin Contact DTOs, handlers, routes, and locale keys.
- [x] T007 Add typed Core internal Contact operation types and invoker dispatch without generic endpoint selection.
- [x] T008 Register `admin_manage`, `service_read`, and `service_manage` capabilities with actor/scope/method metadata.
- [x] T009 Add OpenAPI and Core-internal contract tests, including no `tenant_uuid` override on Admin routes.
- [x] T010 Run capability catalog validation and published/registered/granted denial tests.

## Phase 3 — Framework contract

- [x] T011 Add `runtime/contactfw` types, stable errors, Store/LocalStore, and Runtime factory.
- [x] T012 Implement `DelegatedContactClient` against typed Core operations only.
- [x] T013 Add bootstrap validation for declared Contact consumers.
- [x] T014 Add Framework tests for local/delegated selection, optional non-consumer, startup failure, and no fallback.

## Phase 4 — AI Craft runtime alignment

- [x] T015 Add AI Craft local Contact/ContactIdentity persistence and `FrameworkContactLocalStore`.
- [x] T016 Add `BindFrameworkContact` and inject ContactRuntime into application dependencies.
- [x] T017 Add candidate resolution and explicit create/bind services; preserve `AICraftChannelContact` as candidate-only.
- [ ] T018 Add local/delegated integration tests, including `CONTACT_IDENTITY_NOT_FOUND` and `CONTACT_DELEGATE_UNAVAILABLE`.

## Phase 5 — AI Craft business migration

- [x] T019 Add `primary_contact_uuid` validation to bulk-order create/update and persistence.
- [ ] T020 Add AI Craft ContactAssignment for business roles and replace raw designer responsibility fields.
- [ ] T021 Replace designer input UI with localized Contact selection, explicit regular create, and explicit temporary create.
- [ ] T022 Generate historic-order remediation inventory and block responsibility-dependent transitions for unresolved records.
- [ ] T023 Perform authenticated admin, local plugin, delegated plugin, and browser acceptance runs; document unresolved operational prerequisites.

## Phase 6 — Customer type and primary contact contract

- [x] T024 Require `person|company` in Core basic Customer creation and return type plus `primary_contact_uuid` in list/get/create and Framework selector DTOs.
- [x] T025 Create Customer, membership, and primary natural-person Contact in one transaction; require explicit contact data for a company, and copy person data when omitted. Verify rollback on Contact validation failure.
- [x] T026 Make Core Shopify identity resolution create the person Customer, membership, and primary Contact atomically, and return the original UUIDs on repeated resolution.
- [x] T027 Add typed Contact `email` and `phone` to Core/Framework contracts.
- [x] T027A Add narrow idempotent repair for a verified plugin-attested person identity with a missing primary-contact pointer; reuse one active Contact or create one, and reject ambiguity/stale references.
- [x] T028 Provide a preview and explicit execution workflow for legacy Customer type/contact remediation. Never run remediation during login, reads, or a standard migration.
- [ ] T029 Run installed-plugin delegated/STS and actual Shopify register/login acceptance, including repeat login and failed Contact creation.

## Phase 7 — Core admin customer ingress

- [x] T030 Register and group the Customer menu; incrementally preserve existing permissions/grants in centralized migration.
- [x] T031 Add person/company creation modal and live Admin API authorization, retaining atomic Customer/membership/primary Contact creation.
- [x] T032 Add bounded CSV previews, immutable input version/checksum, creator/tenant isolation, concurrent idempotent commit, durable row results and audit.
- [x] T033 Add import recovery/export and customer list pagination; verify focused frontend, service, permission and PostgreSQL HTTP tests.
- [ ] T034 Verify the deployed production browser after migration, publication and explicit role grants; preserve actual request/Trace evidence.

## Phase 8 — Core primary-contact integrity and explicit legacy repair (2026-10-11)

- [x] T035 Add explicit primary-reference compare/switch, membership locking, role normalization, transactional audit and protection against deactivating the current primary Contact.
- [x] T036 Recheck live per-operation Contact Admin permissions; add tenant/customer reset, stale-response guards and modal invalidation to detail UI.
- [x] T037 Share Unicode field validation across Customer/Contact create/update and CSV preview/commit; verify 43/128/129 Chinese-character boundaries.
- [x] T038 Persist creator/tenant-scoped repair preview/checksum/source fingerprint, explicit type/contact selection and atomic idempotent commit; prevent use by CSV endpoints.
- [x] T039 Publish primary_manage with user-JWT Admin and typed core_internal API-key/STS planes; keep repair Admin-only. Update OpenAPI, Framework handoff and current AI Craft code status.
- [ ] T040 Deploy updated Core/frontend, run migration/catalog publication against the intended instance and verify required role grants. Repeat actual production browser and AI Craft order acceptance. Local tests do not complete this deployment gate.

T017/T019 reflect reviewed current AI Craft code: ContactCandidateService Resolve/CreateFromCandidate/BindCandidate and bulk-order ValidateCustomerContacts. T018/T020/T021/T022/T023 remain separate integration/business/browser gates; their full evidence is not inferred from model or UI existence.
