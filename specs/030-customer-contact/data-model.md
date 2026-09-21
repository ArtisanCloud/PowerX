# 030 Customer Contact: Authoritative Data Model

## Core tables

### `customer_contacts`

| Field | Rule |
| --- | --- |
| `uuid` | Stable `contact_uuid`; public identity. |
| `tenant_uuid` | Required; all access is tenant-scoped. |
| `customer_uuid` | Required; must be an active Customer membership in `tenant_uuid`. |
| `display_name` | Required human-readable primary label. |
| `given_name` / `family_name` | Optional name parts. |
| `status` | `active`, `inactive`, or `temporary`. |
| `roles` | Controlled JSON array: `primary`, `legal_representative` only in v1. |
| `tags` | Constrained JSON array defined by FR-010. |
| `metadata` | Non-identity extension data; no business role fallback. |

Indexes: `tenant_uuid + customer_uuid + status`, `tenant_uuid + customer_uuid + display_name`, and the UUID primary lookup. All lookups still include tenant and Customer predicates even when UUID is indexed.

### `customer_contact_identities`

| Field | Rule |
| --- | --- |
| `uuid` | Stable identity UUID; auditable/addressable object. |
| `tenant_uuid` / `customer_uuid` / `contact_uuid` | Required triple ownership fields. |
| `channel` | Controlled machine identifier such as `email`, `wecom`, or `shopify`. |
| `external_subject` | Required stable external ID, normalized by declared channel contract. |
| `status` | `active` or `inactive`. |
| `verified_at` | Optional channel verification timestamp. |
| `metadata` | Channel metadata without secrets or credential material. |

Unique active binding: `(tenant_uuid, channel, external_subject)`. The identity write transaction loads the Contact under `(tenant_uuid, customer_uuid, contact_uuid)` before persisting.

## Deliberate separation

`customer_auth_identities` proves a Customer's login/authentication identity. `customer_contact_identities` identifies a Customer-associated person for communication. Neither table refers to the other, and no migration reinterprets one as the other.

## AI Craft-owned relations

`ai_craft_contact_assignments` is a plugin-domain table with `tenant_uuid`, `customer_uuid`, `contact_uuid`, `business_role`, status, and audit fields. Its `business_role` vocabulary owns `designer`, `purchaser`, and `finance`. It validates the triple through ContactRuntime before write.

`AICraftChannelContact` remains a candidate/sync record. It may retain the source channel subject, but a selected candidate is usable by an order only after explicit Contact resolution, bind, or create has returned `contact_uuid`.

## Migration and existing data

No name/email/channel candidate may be guessed into `primary_contact_uuid`. The migration creates a remediation inventory for legacy orders. New order writes are immediately strict. Existing incomplete orders remain readable for remediation but fail closed at responsibility-dependent transitions until an operator explicitly selects a Contact.
