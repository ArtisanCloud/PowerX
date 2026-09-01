# Quickstart: Metadata Governance

## Prerequisites

- Working branch: `029-metadata-governance`.
- PostgreSQL and Redis available for normal PowerX development.
- Backend dependencies ready for Go 1.26.7.
- Web Admin dependencies installed for Nuxt 4.

## Backend Development Flow

1. Implement models and migration registration.

   Models belong under:

   ```text
   backend/pkg/corex/db/persistence/model/metadata/
   ```

   Register them in the centralized CoreX migration flow:

   ```text
   backend/pkg/corex/db/database/migration.go
   ```

   Do not run migration from backend startup.

2. Run explicit migration.

   Use the repository's migration command or Make target once it exists for this branch:

   ```bash
   make migrate-up
   ```

   If the repo uses a different explicit migration command in the current branch, use that command. Do not add runtime AutoMigrate to app startup.

3. Implement repository and service layers.

   Required layers:

   ```text
   backend/pkg/corex/db/persistence/repository/metadata/
   backend/internal/service/metadata/
   backend/internal/dto/metadata/
   ```

   Service tests must cover tenant isolation, uniqueness, status transitions, deletion conflicts, tag binding validator failures, and reference registration rollback.

4. Implement REST and gRPC transports.

   REST:

   ```text
   backend/internal/transport/http/admin/metadata/
   ```

   gRPC:

   ```text
   backend/api/grpc/contracts/powerx/metadata/v1/metadata.proto
   backend/internal/transport/grpc/metadata/
   ```

   Generate and validate proto:

   ```bash
   make proto-gen
   make proto-lint
   ```

5. Add platform capability declarations.

   Add:

   ```text
   backend/config/platform_capabilities/metadata.yaml
   ```

   Then verify:

   ```bash
   make capability-check
   ```

6. Add explicit metadata seed command.

   Seed definitions live under:

   ```text
   backend/config/metadata_governance/
   ```

   Seed may run through:

   - explicit command for development/repair;
   - tenant bootstrap hook for new tenants.

   Seed must not run from backend startup.

## Web Admin Flow

1. Add route and page:

   ```text
   web-admin/app/pages/settings/metadata-governance/index.vue
   ```

2. Add API composable/store/types:

   ```text
   web-admin/app/composables/api/metadata-governance.ts
   web-admin/app/stores/metadata-governance.ts
   web-admin/app/types/metadata-governance.ts
   ```

3. Add locale entries for every visible label, button, empty state, error, toast, validation message, and confirm message.

4. Verify these page states for every tab:

   - loading;
   - no permission;
   - missing selection;
   - empty result;
   - backend error;
   - contract/capability authorization error.

## Plugin/Framework Verification

MVP framework contract must support:

- resolve resource type;
- list dictionary items;
- list taxonomy nodes;
- list tags;
- replace tag bindings.

Delegated mode must call governed PowerX metadata capabilities. It must not read plugin private defaults as fallback.

Local mode must use canonical seed-derived data and fail initialization when required seed definitions are missing.

## Acceptance Commands

Run backend tests:

```bash
cd backend
go test ./internal/service/metadata/... ./pkg/corex/db/persistence/repository/metadata/... ./internal/tests/http/admin/metadata/...
```

Run capability verification:

```bash
make capability-check
```

Run Web Admin tests:

```bash
cd web-admin
npm run test
```

Manual smoke path:

1. Run explicit migration.
2. Run explicit metadata seed for a test tenant.
3. Start backend and web-admin.
4. Open `设置 > 元数据治理`.
5. Create a dictionary namespace and five items.
6. Disable one item and verify it is not selectable for new data.
7. Create taxonomy nodes and verify invalid move is rejected.
8. Register a resource type without validator and verify tag binding write is rejected.
9. Register a resource type with validator and verify tag binding replace succeeds.
10. Remove metadata permission from a user and verify manage buttons are unavailable.
