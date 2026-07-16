# Metadata Governance

Metadata Governance is the CoreX-owned mechanism for tenant-scoped dictionaries, taxonomies, tags, resource types, protected references, and explicit metadata seed flows.

Planning sources:

- `specs/029-metadata-governance/spec.md`
- `specs/029-metadata-governance/plan.md`
- `specs/029-metadata-governance/data-model.md`
- `specs/029-metadata-governance/quickstart.md`

Runtime rules:

- Metadata definitions are CoreX data, not plugin-private fallback data.
- Migrations run only through the explicit database migration command.
- Metadata seed is part of the explicit `make seed` / `cmd/database seed` flow and tenant bootstrap hooks, never backend startup.
- Business references use object UUIDs.
- User-visible labels come from i18n maps; machine identifiers are not primary UI labels.
- Dictionaries and tags are flat lists. They must not be used to encode parent-child category paths.
- Categories are represented by taxonomy nodes. Taxonomy nodes are hierarchical and must be linked by `parent_uuid`.

## Metadata Shapes

| Shape | Structure | Use case |
| --- | --- | --- |
| Dictionary | Namespace -> flat items | Status, source, priority, visibility and other enumerations. |
| Taxonomy | Taxonomy -> tree nodes | Knowledge categories, customer industries, media categories and other hierarchical categories. |
| Tag | Flat label pool | Resource labels used for filtering, grouping, and binding statistics. |
| Resource Type | Flat governed object definition | Business objects that can be tagged, referenced, and validated. |

## Plugin Consumption Contract

Plugins consume metadata in two explicit modes.

### Delegated Mode

Delegated plugins call PowerX Core through governed capability paths. The plugin backend must use `/api/v1/tenant/invocations` and these capability IDs:

- `com.corex.metadata.resource_type.read`
- `com.corex.metadata.dictionary.read`
- `com.corex.metadata.taxonomy.read`
- `com.corex.metadata.tag.read`
- `com.corex.metadata.tag.manage`

The invocation payload uses the standard REST selector shape:

```json
{
  "method": "GET",
  "endpoint": "/api/v1/admin/metadata/tags",
  "query": {
    "resource_type": "metadata.demo_resource",
    "locale": "zh-CN"
  }
}
```

Replacing tag bindings uses:

```json
{
  "method": "PUT",
  "endpoint": "/api/v1/admin/metadata/tag-bindings",
  "body": {
    "resource_type": "metadata.demo_resource",
    "resource_uuid": "<business-object-uuid>",
    "tag_uuids": ["<tag-uuid>"]
  }
}
```

Delegated mode must not read plugin-private seed files as fallback. Missing capability, missing tenant registration, missing permission, or missing resource validator must return a visible error.

### Local Mode

Local plugin development must initialize from the canonical seed file under:

```text
backend/config/metadata_governance/seed.yaml
```

If the file is missing, invalid, or does not contain canonical definitions, plugin metadata initialization must fail. It must not return empty lists or invent default dictionaries/tags.

### Seed and Bootstrap

Use the standard seed entrypoint for development, repair, and local verification:

```bash
make seed
```

`make seed` runs CoreX database seed, metadata governance seed, enterprise baseline metadata seed, and Capability Registry seed. Metadata seed is tenant-scoped and is applied to all active tenants.

The seed files are:

```text
backend/config/metadata_governance/seed.yaml
backend/config/metadata_governance/enterprise_seed.yaml
```

Use the low-level command only when targeting one tenant or one seed file during debugging:

```bash
cd backend
go run ./cmd/metadata_seed -tenant-uuid <tenant_uuid> -seed config/metadata_governance/seed.yaml
```

For validation without writes:

```bash
cd backend
go run ./cmd/metadata_seed -tenant-uuid <tenant_uuid> -seed config/metadata_governance/seed.yaml -dry-run -require-canonical
```

New tenant bootstrap calls the same seed service through an explicit hook. Backend startup does not run metadata seed.

## Quickstart Smoke Notes

The implemented smoke path uses explicit operator commands:

```bash
make migrate
make seed
```

Use `make metadata-seed-validate METADATA_SEED_TENANT_UUID=<tenant_uuid>` for the same canonical seed validation through Make.

Backend startup does not run AutoMigrate or AutoSeed. New tenant provisioning calls metadata bootstrap through the tenant create/upsert hook; it returns an error when canonical seed fails, but the current tenant service path is not a full transaction rollback boundary.

UI smoke requires backend and Web Admin to be running with an admin user that has the corresponding read/manage permissions:

- `metadata.dictionary:read`
- `metadata.dictionary:manage`
- `metadata.taxonomy:read`
- `metadata.taxonomy:manage`
- `metadata.tag:read`
- `metadata.tag:manage`
- `metadata.resource_type:read`
- `metadata.resource_type:manage`

Open `Settings > Metadata Governance`, then verify dictionaries, taxonomies, tags, and resource types each show loading, empty, no-permission, missing-selection, and backend-error states distinctly.

## Admin Create UX Contract

The admin create modal is context driven:

- The top-level page shows the metadata shape tabs first. Filters and create actions live inside the active tab workspace.
- Dictionary, taxonomy, and resource type tabs filter by module; the tag tab filters by resource type.
- Dictionary items inherit the currently selected dictionary namespace.
- Taxonomy nodes inherit the currently selected taxonomy and use a parent-node selector.
- Tags select an existing resource type; resource type is not typed manually.
- Top-level dictionaries, taxonomies, and resource types may introduce a new module identifier and also show known modules as quick choices.
- Child creates never show a module field because they inherit the selected parent context.
- Module labels shown to users must be localized business names. Raw `corex.*` module identifiers are submission values or diagnostics, not primary labels.
- Localized name and description editing must scale to many languages. The form always shows required `zh-CN` fields, then edits other languages one at a time through an inline search box plus scrollable locale buttons. Do not use a popover/dropdown that can cover the submit buttons. Filled optional locales are shown as status chips.

Do not reintroduce a page-level global create button, fixed Chinese/English-only fields, or module inputs for child objects.
