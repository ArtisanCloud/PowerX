# 030 Contact Core Internal Contract

## Strong typed binding

The delegated Framework adapter binds this interface, not a generic endpoint proxy:

```go
type DelegatedContactClient interface {
    ListByCustomer(ctx context.Context, in ListByCustomerInput) (ContactPage, error)
    Get(ctx context.Context, in GetContactInput) (*Contact, error)
    ResolveIdentity(ctx context.Context, in ResolveContactIdentityInput) (*ContactIdentityResolution, error)
    Create(ctx context.Context, in CreateContactInput) (*Contact, error)
    Update(ctx context.Context, in UpdateContactInput) (*Contact, error)
    BindIdentity(ctx context.Context, in BindContactIdentityInput) (*ContactIdentity, error)
}
```

Each input carries only Contact business fields and scoped UUIDs. `tenant_uuid`, actor, plugin ID, request ID, and trace ID are sourced from authenticated Core context or explicit typed invocation envelope, never from a free-form proxy payload. The interface contains no `endpoint`, `method`, raw headers, or untyped request body.

For both `ResolveIdentityInput` and `BindIdentityInput`, the channel field is exactly `channel_dictionary_item_uuid`. It must be the UUID of an enabled item in the authenticated tenant's `corex.customer.contact_identity_channel` namespace. `channel` code/string is removed from this contract and must be rejected rather than translated.

## Capability matrix

| Capability ID | Binding | Actor | Scope |
| --- | --- | --- | --- |
| `com.corex.customer.contacts.admin_manage` | Admin REST plus Core internal admin implementation | `admin_user` | tenant, selected Customer |
| `com.corex.customer.contacts.service_read` | Core internal typed read operations | `service_actor` | granted tenant, selected Customer |
| `com.corex.customer.contacts.service_manage` | Core internal typed create/update/bind operations | `service_actor` or explicit delegated/OBO actor | granted tenant, selected Customer |

`admin_manage` REST bindings are user-JWT-only and are never STS direct routes. `service_*` calls require capability publication, tenant registration, credential grant, method/operation matching, and audit dimensions. `sts_direct` route allowlisting is not authorization.

## Framework Host API Key contract

Framework local Host simulation uses the same service capability authorization
as delegated runtime. `PX_GATEWAY_API_KEY` must receive an explicit API Key
grant; an Admin RBAC permission does not grant it. The Core declarations are:

| Capability ID | Fixed binding | Typed operation | API Key scope |
| --- | --- | --- | --- |
| `com.corex.customer.accounts.service_read` | `core://customer/accounts` | `list` | `_scope.customer.accounts.service_read` |
| `com.corex.customer.accounts.service_manage` | `core://customer/accounts` | `create`, `update` | `_scope.customer.accounts.service_manage` |
| `com.corex.customer.contacts.service_read` | `core://customer/contacts` | `list_by_customer`, `get`, `resolve_identity` | `_scope.customer.contacts.service_read` |
| `com.corex.customer.contacts.service_manage` | `core://customer/contacts` | `create`, `update`, `bind_identity` | `_scope.customer.contacts.service_manage` |

The Account binding accepts typed `list`, `create`, and `update` requests.
`update` uses presence-aware optional profile fields, rejects type/primary-contact
changes, and never changes Contact or login identities. The complete update
contract and verification commands are in
`docs/contracts/customer-service-update.md`. `create` accepts `type=person|company` (omitted/empty defaults to `person`) and atomically
produces basic Customer data, a tenant membership, and a primary natural-person
Contact. A person without explicit `primary_contact` copies their name and
available email/phone; a company requires explicit natural-person
`primary_contact`. The response contains `type` and `primary_contact_uuid`.
It does not create a password-login identity. The former free `method` plus
Admin endpoint payload is invalid and has no compatibility path.

PowerXPlugin Framework Lab local/API Key Customer list and Contact list were
reported successful with a screenshot on 2026-09-24. That observation does not
verify an installed plugin's delegated/STS credentials, Contact writes, or AI
Craft order persistence. The handoff checklist is in
`docs/guides/features/030-customer-contact/ai-craft-alignment.md`.

## Target Admin REST surface

These routes are implemented Core Admin contracts:

| Method | Route | Operation |
| --- | --- | --- |
| GET | `/api/v1/admin/customers/:customer_uuid/contacts` | List Contacts for Customer |
| POST | `/api/v1/admin/customers/:customer_uuid/contacts` | Explicitly create Contact |
| GET | `/api/v1/admin/customers/:customer_uuid/contacts/:contact_uuid` | Get scoped Contact |
| PATCH | `/api/v1/admin/customers/:customer_uuid/contacts/:contact_uuid` | Update scoped Contact |
| POST | `/api/v1/admin/customers/:customer_uuid/contacts:resolve-identity` | Resolve only; no creation |
| POST | `/api/v1/admin/customers/:customer_uuid/contacts/:contact_uuid/identities` | Explicitly bind identity |

The authenticated tenant is derived from user JWT/membership. The route, query, and body do not accept `tenant_uuid`. List default page size is 20; maximum page size is 100.

## Framework contract

`runtime/contactfw` defines a `Store` matching the six typed operations and a `LocalStore` that implements it. `Runtime` uses the Framework provider factory to select local or delegated once. A consuming plugin must resolve `Store()` in bootstrap and fail there if unavailable. A non-consuming plugin does not construct ContactRuntime.

`CONTACT_DELEGATE_UNAVAILABLE` is returned when a delegated operation cannot reach or validate Core. The adapter never attempts a local query after that error.

## Framework implementation request

This is a Core contract change for the Framework owner; it is not implemented in this repository's Framework checkout.

1. Replace `ContactIdentity.Channel string` with `ContactIdentity.ChannelDictionaryItemUUID string` in `runtime/contactfw` DTOs and local schema.
2. Replace `channel` with required `channel_dictionary_item_uuid` in `ResolveIdentityInput` and `BindIdentityInput`; do not support both fields or translate a code to UUID.
3. In delegated mode, send only the new JSON field to Core. An old payload must fail validation visibly.
4. In local mode, validate the item against the same tenant-scoped `corex.customer.contact_identity_channel` dictionary contract, or return `CONTACT_CHANNEL_DICTIONARY_INVALID` when no approved metadata binding is available. It must not accept a free-text channel.
5. Treat persisted identities without a channel dictionary item UUID as `CONTACT_IDENTITY_CHANNEL_MIGRATION_REQUIRED`; provide an explicit operator migration path, never an inferred conversion from old channel text.

## Default type and primary-contact invariant (2026-09-26)

Customer writes and verified identity resolution persist an empty historical type as `person`, preserve explicit `company`, and atomically bind the sole active Contact or create a person Contact using saved Customer details when none exists. Stale pointers and multiple active candidates fail and roll back the type normalization too. Company creation still requires a natural-person contact; no company-name-derived person is created. List/get/Contact queries never repair data. Repeated resolution preserves UUIDs. Repeated create requests without an identity or idempotency key remain separate explicit creations.

## Customer external authentication identities (2026-09-27)

These are distinct from ContactIdentity. The new fixed `core://customer/external-identities` `INVOKE` binding exposes `lookup`/`list_by_customer` under `com.corex.customer.external_identities.service_read` and `bind`/`create_and_bind` under `service_manage`. See `docs/contracts/customer-external-identity-management.md` for typed inputs, explicit API Key plugin ownership, STS instance grants, transaction/concurrency guarantees, and display-label semantics. Existing login Resolve retains its side effects and is not used for management inspection.
