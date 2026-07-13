# Research: Metadata Governance

## Decision: Implement as CoreX module `corex.metadata`

**Rationale**: Metadata governance is a platform authority used by CoreX modules, plugins, Agent flows, and admin pages. It must be tenant-scoped and enforced by PowerX Core RBAC/capability systems, so it belongs in CoreX rather than a plugin.

**Alternatives considered**:

- Plugin implementation: rejected because plugins cannot be the canonical authority for platform metadata.
- Per-module metadata stores: rejected because it preserves the current duplication problem.

## Decision: Use separate definition tables, not one generic metadata table

**Rationale**: Dictionaries, taxonomies, tags, resource types, and references have different lifecycle rules, uniqueness constraints, and UI behavior. Separate tables keep validation strict and make constraints testable.

**Alternatives considered**:

- Single polymorphic `metadata_objects` table: rejected because it hides incompatible semantics behind generic fields and weakens indexes and validation.
- JSON-only registry: rejected because deletion protection, filtering, and references require relational queries.

## Decision: Store i18n values as JSONB maps with required `zh-CN`

**Rationale**: User-visible names and descriptions must not be hard-coded or replaced by raw codes. JSONB maps allow admin editing across locales while keeping one record per metadata object.

**Alternatives considered**:

- Separate translation table: deferred because MVP needs simpler CRUD and admin editing; can be revisited if translation workflow grows.
- Single-language text fields: rejected because it violates i18n requirements.

## Decision: Missing requested locale is visible, not silent

**Rationale**: Admin pages need to remain usable even when a locale is missing, but the absence must be explicit. APIs should return `display_locale_missing` and the UI should show a translation-missing state.

**Alternatives considered**:

- Fail the list API when a locale is missing: rejected because it makes admin cleanup difficult.
- Show code or UUID as the label: rejected by UI and i18n rules.
- Silent fallback to `zh-CN`: rejected because it hides data quality problems.

## Decision: Tag binding writes require object validators

**Rationale**: Tag bindings are polymorphic and cannot rely on a single DB foreign key. Without a resource type object validator, the service cannot prove the resource exists or belongs to the current tenant.

**Alternatives considered**:

- Validate only UUID format: rejected because it permits dangling and cross-tenant bindings.
- Async repair invalid bindings: rejected because delete protection and usage counts become untrustworthy.

## Decision: Protected references are explicit records

**Rationale**: Deletion protection must not scan every business table. `metadata_references` gives a stable conflict source and reference summary for dictionary items and taxonomy nodes; tag bindings already provide direct reference records.

**Alternatives considered**:

- On-demand scans of business modules: rejected because it does not scale and requires module-specific coupling.
- Reference counters only: rejected because counters are display hints, not authoritative conflict records.

## Decision: Reference registration is part of adopting module write consistency

**Rationale**: Once a module adopts metadata governance, business writes and reference registration must succeed or fail together. Partial success would make hard-delete protection unreliable.

**Alternatives considered**:

- Async reference registration: rejected for MVP because it creates deletion races.
- Warning-only registration failures: rejected because it hides data corruption.

## Decision: Seed runs only through explicit command or tenant bootstrap hook

**Rationale**: Runtime startup must not create tables or seed data. Explicit command supports development/repair; tenant bootstrap hook supports new tenant provisioning.

**Alternatives considered**:

- Backend startup AutoSeed: rejected because it couples runtime availability to data mutation.
- Admin page seed button: rejected for MVP because initialization is operational/bootstrap behavior, not normal metadata editing.

## Decision: Capability declarations are business authorization units

**Rationale**: Metadata APIs should expose read/manage boundaries for dictionary, taxonomy, tag, and resource type domains. Capabilities should not be raw route IDs. STS direct behavior must be derived from formal capability declarations and blocklist rules.

**Alternatives considered**:

- One capability per REST route: rejected because it creates unreadable grants and UI noise.
- Validator-only allowlisting: rejected because it bypasses capability governance.

## Decision: MVP excludes existing business module migration

**Rationale**: The first release must make the governance platform usable and testable before migrating customer, knowledge, agent, media, or other module data. Each adopting module must provide a mapping and migration plan later.

**Alternatives considered**:

- Migrate customer module in MVP: rejected because it expands scope and mixes platform infrastructure with business data migration.
- Migrate multiple modules: rejected due to high regression risk.
