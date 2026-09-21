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
- [ ] T009 Add OpenAPI and Core-internal contract tests, including no `tenant_uuid` override on Admin routes.
- [ ] T010 Run capability catalog validation and published/registered/granted denial tests.

## Phase 3 — Framework contract

- [x] T011 Add `runtime/contactfw` types, stable errors, Store/LocalStore, and Runtime factory.
- [x] T012 Implement `DelegatedContactClient` against typed Core operations only.
- [x] T013 Add bootstrap validation for declared Contact consumers.
- [x] T014 Add Framework tests for local/delegated selection, optional non-consumer, startup failure, and no fallback.

## Phase 4 — AI Craft runtime alignment

- [x] T015 Add AI Craft local Contact/ContactIdentity persistence and `FrameworkContactLocalStore`.
- [x] T016 Add `BindFrameworkContact` and inject ContactRuntime into application dependencies.
- [ ] T017 Add candidate resolution and explicit create/bind services; preserve `AICraftChannelContact` as candidate-only.
- [ ] T018 Add local/delegated integration tests, including `CONTACT_IDENTITY_NOT_FOUND` and `CONTACT_DELEGATE_UNAVAILABLE`.

## Phase 5 — AI Craft business migration

- [ ] T019 Add `primary_contact_uuid` validation to bulk-order create/update and persistence.
- [ ] T020 Add AI Craft ContactAssignment for business roles and replace raw designer responsibility fields.
- [ ] T021 Replace designer input UI with localized Contact selection, explicit regular create, and explicit temporary create.
- [ ] T022 Generate historic-order remediation inventory and block responsibility-dependent transitions for unresolved records.
- [ ] T023 Perform authenticated admin, local plugin, delegated plugin, and browser acceptance runs; document unresolved operational prerequisites.
