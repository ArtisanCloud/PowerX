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

## Capability matrix

| Capability ID | Binding | Actor | Scope |
| --- | --- | --- | --- |
| `com.corex.customer.contacts.admin_manage` | Admin REST plus Core internal admin implementation | `admin_user` | tenant, selected Customer |
| `com.corex.customer.contacts.service_read` | Core internal typed read operations | `service_actor` | granted tenant, selected Customer |
| `com.corex.customer.contacts.service_manage` | Core internal typed create/update/bind operations | `service_actor` or explicit delegated/OBO actor | granted tenant, selected Customer |

`admin_manage` REST bindings are user-JWT-only and are never STS direct routes. `service_*` calls require capability publication, tenant registration, credential grant, method/operation matching, and audit dimensions. `sts_direct` route allowlisting is not authorization.

## Target Admin REST surface

These routes are planned contracts, not yet present in the current runtime:

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
