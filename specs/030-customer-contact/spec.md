# Feature Specification: Customer Contact

**Feature Branch**: `030-customer-contact`  
**Created**: 2026-09-20  
**Status**: In progress; Core persistence, Admin transport, and Core internal binding implemented  
**Input**: Extend PowerX Customer with tenant-scoped natural-person contacts and channel identities, then expose the same strict contract through PowerXPlugin local and delegated runtimes.

## Scope and non-goals

`Customer` remains the customer subject, login identity, and tenant membership. `Contact` is a natural person that a tenant associates with one Customer for business communication. `ContactIdentity` is that Contact's stable identifier in a channel such as email, WeCom, or Shopify.

This feature does not replace `customer_auth_identities`, create industry-specific people records, infer a Contact from a display name, or make channel candidate data authoritative. AI Craft responsibilities such as `designer`, `purchaser`, and `finance` are plugin-domain assignments, not Contact roles.

## User Scenarios & Testing

### User Story 1 - Manage a Customer's contacts (P1)

As a tenant administrator, I can list, create, read, and update Contacts below a selected Customer, so that a business order refers to a verified natural person rather than copied text.

**Independent test**: Create two Customers in one tenant and a Contact for the first. The Contact is listable and updatable under the first Customer, but is rejected when supplied under the second.

**Acceptance scenarios**:

1. Given an active tenant Customer, when an authorized administrator creates a Contact with a display name and explicit state, then the result contains `contact_uuid`, `tenant_uuid`, and the submitted `customer_uuid`.
2. Given a Contact under a Customer, when a caller supplies the Contact UUID with another Customer UUID, then the operation fails with `CONTACT_CUSTOMER_MISMATCH` and no record changes.
3. Given a Customer not active in the request tenant, when any Contact write is requested, then it fails before a Contact or identity row is written.
4. Given a Contact is updated, when a field is absent from a PATCH request, then it is unchanged; explicit empty values follow the field validation rule.

### User Story 2 - Resolve a channel identity without implicit creation (P1)

As an administrator or authorized plugin flow, I can resolve a Contact by a stable channel identity for a specified Customer, so that an incoming message can be associated without guessing a person.

**Independent test**: Resolve a known `wecom` identity under its Customer and receive the Contact. Resolve an unknown identity and receive `CONTACT_IDENTITY_NOT_FOUND`; no Contact is created.

**Acceptance scenarios**:

1. A resolution input requires `customer_uuid`, `channel`, and `external_subject`; a display name is never an identity key.
2. The same `(tenant_uuid, channel, external_subject)` cannot bind to two Contacts.
3. A channel identity found for a different Customer is not returned as a match for the requested Customer.
4. Creating a regular or `temporary` Contact is a separate explicit command and writes structured `creation_intent=explicit_create` or `explicit_temporary` audit data.

### User Story 3 - Use a governed universal role vocabulary (P1)

As a platform consumer, I can see only cross-business Contact relationship roles, while plugin-specific responsibilities remain owned by the plugin.

**Independent test**: A Contact can be assigned `primary`; creating or updating it with `designer` fails. AI Craft can separately persist `designer` against an existing Contact UUID.

**Acceptance scenarios**:

1. Initial Contact role vocabulary is exactly `primary` and `legal_representative`.
2. Contact tags are an optional constrained array: at most 20 entries, each 1-64 characters, total UTF-8 payload at most 1,024 bytes, and each entry matches `^[a-z0-9][a-z0-9._-]{0,63}$`.
3. Roles and tags are validated at Core and local-store boundaries; neither boundary accepts arbitrary free text.

### User Story 4 - Select exactly one runtime mode (P1)

As a plugin developer, I can consume one `contactfw` contract in local or delegated mode, without business code selecting a transport or silently reading another store.

**Independent test**: A plugin declaring Contact starts in local mode with a `LocalStore`, and in delegated mode with a typed Core client. A declared consumer with the chosen adapter absent fails during bootstrap. A delegated client failure returns `CONTACT_DELEGATE_UNAVAILABLE` and never queries local persistence.

**Acceptance scenarios**:

1. Framework introduces `runtime/contactfw`, separate from `runtime/customerfw`.
2. A plugin that neither declares nor consumes Contact may start without constructing ContactRuntime.
3. A plugin that consumes Contact must resolve `ContactRuntime.Store()` during bootstrap; delayed method-level resolution alone is insufficient.
4. The delegated adapter only calls the strong typed Core internal Contact binding; it cannot choose arbitrary endpoint, method, or body fields.

### User Story 5 - Make AI Craft orders contact-referential (P2)

As an AI Craft operator, I choose or explicitly create a Contact before creating a bulk order, so that the order stores only `customer_uuid` and `primary_contact_uuid`.

**Independent test**: A channel candidate is selected, resolved, explicitly bound or created, then used to create an order. An unmatched candidate presents explicit create choices; an order cannot be created from a raw designer string.

**Acceptance scenarios**:

1. `AICraftChannelContact` remains a synchronization/candidate source, not Contact master data.
2. AI Craft verifies `primary_contact_uuid` through ContactRuntime before order persistence.
3. AI Craft stores its responsibilities through a plugin-owned ContactAssignment relation keyed by `tenant_uuid`, `customer_uuid`, `contact_uuid`, and `business_role`.
4. Existing orders without a manually confirmed Contact are marked pending remediation and cannot advance through actions requiring a responsible Contact.

## Functional Requirements

- **FR-001**: Core MUST provide `Contact` and `ContactIdentity` as separate business objects from Customer and Customer authentication identities.
- **FR-002**: Every Contact and ContactIdentity MUST have a stable UUID; all public, audit, event, relationship, and plugin references MUST use UUIDs.
- **FR-003**: Every Contact and ContactIdentity query and write MUST be constrained by authenticated `tenant_uuid`, `customer_uuid`, and the applicable object UUID.
- **FR-004**: Core MUST verify that `customer_uuid` has an active membership in the tenant before any Contact read or write.
- **FR-005**: Core MUST reject a Contact/Customer mismatch with stable `CONTACT_CUSTOMER_MISMATCH`; it MUST NOT re-parent, infer, or translate the request.
- **FR-006**: Core MUST provide create, get, update, list-by-customer, resolve-identity, and bind-identity typed operations.
- **FR-007**: Resolve identity MUST return `CONTACT_IDENTITY_NOT_FOUND` on a miss and MUST NOT create a Contact, Identity, Customer, or membership.
- **FR-008**: `temporary` Contact status MUST be accepted only on an explicit create operation and audited with its creation intent.
- **FR-009**: Core Contact roles MUST use the controlled universal vocabulary; AI Craft business roles MUST be stored only in AI Craft data.
- **FR-010**: Core MUST validate tag count, size, and grammar as specified by User Story 3.
- **FR-011**: Admin CRUD routes MUST use user JWT, tenant membership, RBAC, and the Contact administration capability; a plugin STS token MUST NOT call `/api/v1/admin/*`.
- **FR-012**: Service-actor reads and writes MUST use separately declared, grant-checked Contact capabilities and a Core internal typed binding.
- **FR-013**: A generic gateway request containing an arbitrary method, endpoint, or body MUST NOT be the delegated Contact contract.
- **FR-014**: A plugin consuming Contact MUST choose `ProviderMode` once at bootstrap and fail startup when its chosen Contact adapter is unavailable.
- **FR-015**: Delegated runtime failures MUST produce `CONTACT_DELEGATE_UNAVAILABLE` and MUST NOT fall back to a local store.
- **FR-016**: Local stores MUST preserve the same validation, UUID, tenant, Customer ownership, error, and no-implicit-create semantics as Core.
- **FR-017**: All user-visible frontend and backend text introduced by this feature MUST be locale resources; UUIDs are secondary diagnostics only.
- **FR-018**: New AI Craft orders MUST persist `customer_uuid` and `primary_contact_uuid`; they MUST NOT write a legacy designer/person text field.
- **FR-019**: Historical records without a confirmed Contact MUST be surfaced for manual remediation, not automatically backfilled from names, emails, or channel candidates.
- **FR-020**: Contact creates, updates, identity bindings, and status changes MUST be transactionally audited with tenant, actor, request ID, Contact UUID, Customer UUID, and trace ID.

## Key Entities

| Entity | Ownership and purpose |
| --- | --- |
| Customer | Global customer subject; tenant eligibility comes from CustomerMembership. |
| Contact | Tenant-scoped natural person associated with one Customer. |
| ContactIdentity | Tenant-scoped stable channel identity bound to exactly one Contact and Customer. |
| AI Craft ContactAssignment | AI Craft-only responsibility such as designer, purchaser, or finance. |
| AICraftChannelContact | AI Craft-only synchronized candidate source; never authoritative Contact data. |

## Error contract

| Code | Meaning |
| --- | --- |
| `CONTACT_INVALID_ARGUMENT` | Missing/invalid UUID, role, tag, channel, or state. |
| `CONTACT_NOT_FOUND` | Contact is not visible under tenant and Customer scope. |
| `CONTACT_CUSTOMER_MISMATCH` | Contact UUID does not belong to supplied Customer UUID. |
| `CONTACT_CUSTOMER_MEMBERSHIP_INACTIVE` | Customer has no active membership in authenticated tenant. |
| `CONTACT_IDENTITY_NOT_FOUND` | Identity resolution did not find a binding. |
| `CONTACT_IDENTITY_CONFLICT` | Channel identity is already bound in tenant scope. |
| `CONTACT_DELEGATE_UNAVAILABLE` | Delegated Core binding cannot serve the request. |
| `CONTACT_CAPABILITY_FORBIDDEN` | Caller lacks the declared Contact capability or grant. |

## Out of scope

- A customer self-service Contact portal.
- Automatic CRM, Shopify, email, or WeCom synchronization that creates Contact master records.
- Free-form universal role definitions or plugin-specific role registry in Core.
- Automated historical order backfill.
