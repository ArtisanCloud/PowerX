# Data Model: Metadata Governance

## Global Rules

- Every business object table has a stable `uuid`.
- Relationship tables may omit their own `uuid`, but all relationship fields reference target object UUIDs.
- Every tenant-scoped metadata object stores `tenant_uuid`.
- `namespace`, `code`, and `resource_type` are machine identifiers and are not i18n fields.
- User-visible labels use `name_i18n`, `label_i18n`, and `description_i18n` JSONB maps.
- `zh-CN` is required in MVP for all user-visible i18n maps.
- Status values are `enabled`, `disabled`, and `archived`.
- Hard deletion is allowed only when no protected references exist.

## Dictionary Namespace

Table: `metadata_dictionary_namespaces`

Fields:

- `uuid`: stable object UUID.
- `tenant_uuid`: owning tenant UUID.
- `namespace`: immutable machine identifier, for example `corex.customer.level`.
- `module`: owning module, for example `corex.customer`.
- `name_i18n`: JSONB map, requires `zh-CN`.
- `description_i18n`: JSONB map, requires `zh-CN` when provided.
- `status`: `enabled`, `disabled`, or `archived`.
- `created_at`, `updated_at`.

Indexes and constraints:

- Unique: `tenant_uuid + namespace`.
- Indexed: `tenant_uuid + module + status`.

Validation:

- `namespace` cannot be UUID, visible label, or free text.
- `namespace` is immutable after creation.
- Cannot hard delete when child items are referenced.

## Dictionary Item

Table: `metadata_dictionary_items`

Fields:

- `uuid`: stable object UUID.
- `tenant_uuid`: owning tenant UUID.
- `namespace_uuid`: parent dictionary namespace UUID.
- `code`: immutable machine identifier.
- `label_i18n`: JSONB map, requires `zh-CN`.
- `description_i18n`: JSONB map, requires `zh-CN` when provided.
- `sort_order`: integer ordering value.
- `status`: `enabled`, `disabled`, or `archived`.
- `metadata`: JSONB for non-label machine metadata.
- `reference_count`: cached display count only, not authoritative for deletion.
- `created_at`, `updated_at`.

Indexes and constraints:

- Unique: `tenant_uuid + namespace_uuid + code`.
- Indexed: `tenant_uuid + namespace_uuid + status + sort_order`.

Validation:

- `code` must be lowercase snake case and immutable.
- `enabled` items are selectable for new data.
- `disabled` items are readable for history but not selectable for new data.
- Hard delete requires zero authoritative references in `metadata_references`.

## Taxonomy

Table: `metadata_taxonomies`

Fields:

- `uuid`: stable object UUID.
- `tenant_uuid`: owning tenant UUID.
- `namespace`: immutable machine identifier.
- `module`: owning module.
- `name_i18n`: JSONB map, requires `zh-CN`.
- `description_i18n`: JSONB map, requires `zh-CN` when provided.
- `max_depth`: positive integer.
- `status`: `enabled`, `disabled`, or `archived`.
- `created_at`, `updated_at`.

Indexes and constraints:

- Unique: `tenant_uuid + namespace`.
- Indexed: `tenant_uuid + module + status`.

Validation:

- `max_depth` must be >= 1.
- `namespace` is immutable.

## Taxonomy Node

Table: `metadata_taxonomy_nodes`

Fields:

- `uuid`: stable object UUID.
- `tenant_uuid`: owning tenant UUID.
- `taxonomy_uuid`: parent taxonomy UUID.
- `parent_uuid`: nullable parent node UUID.
- `code`: immutable machine identifier.
- `label_i18n`: JSONB map, requires `zh-CN`.
- `description_i18n`: JSONB map, requires `zh-CN` when provided.
- `path`: UUID path, for example `/taxonomy_uuid/root_node_uuid/child_node_uuid`.
- `depth`: integer depth starting at 1 for root nodes.
- `sort_order`: integer ordering value.
- `status`: `enabled`, `disabled`, or `archived`.
- `reference_count`: cached display count only.
- `version`: optimistic lock integer or equivalent timestamp-based concurrency marker.
- `created_at`, `updated_at`.

Indexes and constraints:

- Unique: `tenant_uuid + taxonomy_uuid + code`.
- Unique or conflict-check: `tenant_uuid + taxonomy_uuid + parent_uuid + label_i18n_digest`.
- Indexed: `tenant_uuid + taxonomy_uuid + parent_uuid + sort_order`.
- Indexed: `tenant_uuid + taxonomy_uuid + path`.

Validation:

- Cannot move a node under itself or its descendants.
- Move must recalculate `path` and `depth` for the node and descendants.
- Move must reject depth > taxonomy `max_depth`.
- Concurrent move/update conflicts fail fast and require reload.
- Hard delete requires zero authoritative references in `metadata_references`.

## Tag

Table: `metadata_tags`

Fields:

- `uuid`: stable object UUID.
- `tenant_uuid`: owning tenant UUID.
- `namespace`: immutable tag namespace.
- `resource_type`: immutable resource type string.
- `code`: immutable machine identifier.
- `label_i18n`: JSONB map, requires `zh-CN`.
- `description_i18n`: JSONB map, requires `zh-CN` when provided.
- `color`: optional visual hint.
- `source`: `admin`, `seed`, `plugin`, or `system`.
- `status`: `enabled`, `disabled`, or `archived`.
- `usage_count`: cached display count only.
- `created_at`, `updated_at`.

Indexes and constraints:

- Unique: `tenant_uuid + namespace + resource_type + code`.
- Indexed: `tenant_uuid + resource_type + namespace + status`.

Validation:

- `color` cannot carry business semantics.
- Merge requires same `tenant_uuid` and same `resource_type`.
- Source tag and target tag cannot be the same.
- Hard delete requires zero bindings in `metadata_tag_bindings`.

## Tag Binding

Table: `metadata_tag_bindings`

Fields:

- `tenant_uuid`: owning tenant UUID.
- `tag_uuid`: referenced tag UUID.
- `resource_type`: resource type string.
- `resource_uuid`: target business object UUID.
- `created_by_uuid`: member/user UUID for audit visibility.
- `created_at`.

Indexes and constraints:

- Unique: `tenant_uuid + tag_uuid + resource_type + resource_uuid`.
- Indexed: `tenant_uuid + resource_type + resource_uuid`.
- Indexed: `tenant_uuid + tag_uuid`.

Validation:

- Writes require enabled `metadata_resource_types` record.
- Writes require enabled object validator for the resource type.
- Writes must verify target resource exists and belongs to current tenant.
- Replace operation is transactional: remove omitted tags and add new tags as one consistency boundary.

## Resource Type

Table: `metadata_resource_types`

Fields:

- `uuid`: stable object UUID.
- `tenant_uuid`: owning tenant UUID.
- `resource_type`: immutable machine identifier, for example `customer.account`.
- `module`: owning module.
- `name_i18n`: JSONB map, requires `zh-CN`.
- `description_i18n`: JSONB map, requires `zh-CN` when provided.
- `validator_key`: key used by service registry to find object validator.
- `binding_enabled`: boolean indicating whether tag binding writes are allowed.
- `status`: `enabled`, `disabled`, or `archived`.
- `created_at`, `updated_at`.

Indexes and constraints:

- Unique: `tenant_uuid + resource_type`.
- Indexed: `tenant_uuid + module + status`.

Validation:

- `resource_type` is immutable.
- `binding_enabled=true` requires an active validator registered for `validator_key`.
- Disabling a resource type prevents new tag binding writes but existing bindings remain readable.

## Metadata Reference

Table: `metadata_references`

Fields:

- `tenant_uuid`: owning tenant UUID.
- `metadata_type`: `dictionary_item`, `taxonomy_node`, or other explicit governed type.
- `metadata_uuid`: referenced metadata object UUID.
- `resource_type`: adopting business resource type.
- `resource_uuid`: adopting business object UUID.
- `field_name`: business field that stores the metadata reference.
- `created_at`, `updated_at`.

Indexes and constraints:

- Unique: `tenant_uuid + metadata_type + metadata_uuid + resource_type + resource_uuid + field_name`.
- Indexed: `tenant_uuid + metadata_type + metadata_uuid`.
- Indexed: `tenant_uuid + resource_type + resource_uuid`.

Validation:

- Adopting modules must maintain references in the same write consistency boundary as business data.
- Reference registration failure must fail and roll back the business write.
- Deletion conflict checks use this table as authoritative source for dictionary items and taxonomy nodes.

## Metadata Seed Definition

Storage: `backend/config/metadata_governance/*.yaml`

Fields:

- `version`: seed schema version.
- `module`: owner module.
- `dictionaries`: dictionary namespaces and items.
- `taxonomies`: taxonomies and nodes.
- `resource_types`: resource type definitions.
- `tags`: baseline tags.

Validation:

- Missing required fields fail seed before writes.
- Seed upsert keys:
  - Dictionary namespace: `tenant_uuid + namespace`.
  - Dictionary item: `tenant_uuid + namespace + code`.
  - Taxonomy: `tenant_uuid + namespace`.
  - Taxonomy node: `tenant_uuid + taxonomy_uuid + code`.
  - Tag: `tenant_uuid + namespace + resource_type + code`.
  - Resource type: `tenant_uuid + resource_type`.

## Capability Permission Mapping

Formal capabilities:

| Capability ID | Permission Code | Agent Usable | Risk |
|----------------|-----------------|--------------|------|
| `com.corex.metadata.dictionary.read` | `metadata.dictionary:read` | false | low |
| `com.corex.metadata.dictionary.manage` | `metadata.dictionary:manage` | false | medium |
| `com.corex.metadata.taxonomy.read` | `metadata.taxonomy:read` | false | low |
| `com.corex.metadata.taxonomy.manage` | `metadata.taxonomy:manage` | false | medium |
| `com.corex.metadata.tag.read` | `metadata.tag:read` | false | low |
| `com.corex.metadata.tag.manage` | `metadata.tag:manage` | false | medium |
| `com.corex.metadata.resource_type.read` | `metadata.resource_type:read` | false | low |
| `com.corex.metadata.resource_type.manage` | `metadata.resource_type:manage` | false | medium |

## State Transitions

```text
enabled -> disabled -> enabled
enabled -> archived
disabled -> archived
archived -> disabled
```

Rules:

- `enabled`: visible and selectable for new data.
- `disabled`: visible for history, not selectable for new data.
- `archived`: admin-visible only, not in ordinary selectors.
- Hard delete is a separate operation and requires no protected references.
