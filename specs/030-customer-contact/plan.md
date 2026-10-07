# Implementation Plan: Customer Contact

**Branch**: `030-customer-contact`  
**Status**: In progress; Slices 1–3 and AI Craft local/delegated runtime assembly are implemented and focused-verified. Candidate workflow and order/UI migration remain.

## Technical decisions

1. Contact is a Core Customer subdomain with tenant-scoped records; it is not a Customer login/auth feature.
2. The authoritative Core model is mirrored only by plugin local stores for local mode. Delegated mode uses Core and no local fallback.
3. Framework provides a new `runtime/contactfw` package. It depends on scoped Customer validation but does not modify `customerfw.CustomerContext` or `customer_auth_identities`.
4. Core internal invocation is a typed Contact operation binding. Generic `{method, endpoint, body}` gateway requests are prohibited for this module.
5. AI Craft owns business responsibility assignments and channel candidates; it never owns a competing Contact master.

## Implementation slices

### Slice 1 — Core persistence and invariant service

Implement the Core source of truth before any plugin consumer.

- Add Contact and ContactIdentity GORM models under `backend/pkg/corex/db/persistence/model/customer/` and table constants in `tables.go`.
- Register them in `backend/pkg/corex/db/database/migration.go::migrateCustomerModels`; do not create a scattered migration package or SQL file.
- Add tenant-scoped repositories and `ContactService` under existing Customer package boundaries.
- Validate UUID shape, active Customer membership, triple ownership, roles, tags, and identity uniqueness before write.
- Add transactionally consistent audit/log dimensions: `tenant_uuid`, `customer_uuid`, `contact_uuid`, `actor`, `request_id`, `trace_id`, and `creation_intent` where applicable.

**Exit criteria**: focused model/repository/service tests prove creation, list, mismatch rejection, cross-tenant rejection, identity conflict, identity miss without creation, and temporary-create auditing.

### Slice 2 — Core typed contracts, HTTP, and capabilities

- Add request/response DTOs and target OpenAPI documentation from `contracts/contact-core-internal.md`.
- Add Admin handler/routes under `backend/internal/transport/http/admin/customer/`; all human-visible error rendering uses locale keys.
- Extend `backend/internal/service/customer/capability_invoker.go` with typed Contact dispatch, not endpoint parsing for Contact.
- Register the three Contact capabilities in `backend/config/platform_capabilities/customer.yaml`, regenerate the catalog, and add capability/grant tests.
- Keep `/api/v1/admin/*` user-JWT-only. Define service operations as Core internal typed bindings; do not loosen STS route validation.

**Exit criteria**: route contract tests, Core-internal typed binding tests, `make capability-check`, and denied ungranted-plugin tests pass.

### Slice 3 — Framework `contactfw`

- Add `PowerXPlugin/framework/backend/go/runtime/contactfw/` with entities, input types, stable errors, `Store`, `LocalStore`, `DelegatedContactClient`, and Runtime factory.
- Implement a Core typed delegated client with no raw HTTP/Bearer forwarding and no generic action/endpoint API.
- Add bootstrap validation API so declared consumers call `ContactRuntime.Store()` during startup.
- Test local selection, delegated selection, absent selected adapter startup failure, no-consumer optionality, and delegated-unavailable no-fallback behavior.

**Exit criteria**: framework package tests prove all mode and strong-type constraints without needing an AI Craft business test.

### Slice 4 — AI Craft local and delegated assembly

- Add AI Craft `FrameworkContactLocalStore` and plugin-local Contact/ContactIdentity schema/repositories with Core-equivalent invariants.
- Add `BindFrameworkContact(deps)` adjacent to `BindFrameworkCustomer(deps)` and select the same ProviderMode once.
- Construct ContactRuntime only from AI Craft's explicit Contact consumption declaration; when declared, bootstrap resolves it immediately.
- In delegated mode bind only the Framework typed Contact client; never read AI Craft local Contact records.
- Keep `AICraftChannelContact` as a candidate source and implement explicit resolve/bind/create choices.

**Exit criteria**: local and delegated integration tests cover the same successful and failed channel-candidate flows.

### Slice 5 — AI Craft order migration and UI

- Add `customer_uuid` and `primary_contact_uuid` to new bulk-order create/update contracts; validate through ContactRuntime before persistence.
- Add `ai_craft_contact_assignments` for `designer`, `purchaser`, and `finance` as needed by domain flows.
- Replace the free-text designer field with Contact search/resolve and explicit create/temporary-create actions. Add locale keys for every new label, validation message, and error state.
- Produce an operator remediation inventory for historic orders. Do not infer mapping from text, email, or channels. Block responsibility-dependent actions until manually resolved.

**Exit criteria**: a real AI Craft order flow persists only UUID references and UI/API tests prove unmatched candidates cannot silently create a Contact.

## Delivery order and dependencies

`Slice 1 → Slice 2 → Slice 3 → Slice 4 → Slice 5` is mandatory. Slices 1 and 2 establish the authoritative contract; Slice 3 must not invent a plugin-only variant. AI Craft UI work waits for its local and delegated runtime tests.

## Test and rollout plan

| Layer | Required proof |
| --- | --- |
| Core model/repository | UUID, tenant, Customer ownership, unique identity, tag/role validation |
| Core service | transaction/audit, not found, mismatch, conflict, explicit temporary |
| Core HTTP/capability | Admin RBAC, service grant denial, typed operation allowlist, OpenAPI contract |
| Framework | local/delegated selection, optional non-consumer, startup failure for declared consumer, no fallback |
| AI Craft | candidate resolve, explicit create/bind, order pair validation, historic remediation gate |
| Runtime | migration, capability publication/registration/grant, authenticated admin and delegated calls |

## Risks and mitigations

| Risk | Control |
| --- | --- |
| Contact is confused with Customer authentication | separate models, packages, capabilities, and docs; no conversion path |
| AI Craft leaks domain roles into Core | Core role allowlist and plugin-only assignment table |
| Generic gateway becomes arbitrary proxy | no generic Contact gateway contract; typed Core binding only |
| Delegated outage reads stale local data | `CONTACT_DELEGATE_UNAVAILABLE`, no fallback |
| Historic data guessed incorrectly | remediation inventory and manual binding only |
