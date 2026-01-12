---
description: Execute the implementation plan by processing and executing all tasks defined in tasks.md
---

## User Input

```text
$ARGUMENTS
```

You **MUST** consider the user input before proceeding (if not empty).

## Scoped Execution (T206–T209)

If the user input indicates a task range such as `T206-T209` (or an explicit list containing `T206`, `T207`, `T208`, `T209`), you MUST run a **scoped implementation**:

- Only implement tasks `T206`–`T209` from `specs/011-knowledge-space/tasks.md`
- Do NOT attempt other tasks (e.g. `T210+`) unless the user explicitly expands scope
- Still follow the same checklist and validation rules, but limit code changes to what is required for `T206`–`T209`

### Task Intent (authoritative)

From `specs/011-knowledge-space/tasks.md`:

- **T206**: Add pgvector migrations: `CREATE EXTENSION vector` + `public.knowledge_vectors` (incl. `space_idx` + `embedding_idx`), idempotent.
- **T207**: Add KG assist table migrations: `public.knowledge_kg_nodes` / `public.knowledge_kg_edges` (incl. required indexes), idempotent.
- **T208**: Align `make db-migrate` (`backend/cmd/database/migrate.go` path): run pgvector migrations **only when** `knowledge_space.vector_store.driver=pgvector`; define DSN selection rule (use `pgvector.dsn`, fallback to main DB DSN when empty).
- **T209**: Add migration verification test: run `go run ./cmd/database migrate` against a test Postgres, assert `to_regclass('public.knowledge_vectors')` (when driver=pgvector) and `knowledge_kg_*` exist; rerun to assert idempotency.

## Outline

1. Run `.specify/scripts/bash/check-prerequisites.sh --json --require-tasks --include-tasks` from repo root and parse FEATURE_DIR and AVAILABLE_DOCS list. All paths must be absolute. For single quotes in args like "I'm Groot", use escape syntax: e.g 'I'\''m Groot' (or double-quote if possible: "I'm Groot").

2. **Check checklists status** (if FEATURE_DIR/checklists/ exists):
   - Scan all checklist files in the checklists/ directory
   - For each checklist, count:
     * Total items: All lines matching `- [ ]` or `- [X]` or `- [x]`
     * Completed items: Lines matching `- [X]` or `- [x]`
     * Incomplete items: Lines matching `- [ ]`
   - Create a status table:
     ```
     | Checklist | Total | Completed | Incomplete | Status |
     |-----------|-------|-----------|------------|--------|
     | ux.md     | 12    | 12        | 0          | ✓ PASS |
     | test.md   | 8     | 5         | 3          | ✗ FAIL |
     | security.md | 6   | 6         | 0          | ✓ PASS |
     ```
   - Calculate overall status:
     * **PASS**: All checklists have 0 incomplete items
     * **FAIL**: One or more checklists have incomplete items
   
   - **If any checklist is incomplete**:
     * Display the table with incomplete item counts
     * **STOP** and ask: "Some checklists are incomplete. Do you want to proceed with implementation anyway? (yes/no)"
     * Wait for user response before continuing
     * If user says "no" or "wait" or "stop", halt execution
     * If user says "yes" or "proceed" or "continue", proceed to step 3
   
   - **If all checklists are complete**:
     * Display the table showing all checklists passed
     * Automatically proceed to step 3

3. Load and analyze the implementation context:
   - **REQUIRED**: Read tasks.md for the complete task list and execution plan
   - **REQUIRED**: Read plan.md for tech stack, architecture, and file structure
   - **IF EXISTS**: Read data-model.md for entities and relationships
   - **IF EXISTS**: Read contracts/ for API specifications and test requirements
   - **IF EXISTS**: Read research.md for technical decisions and constraints
   - **IF EXISTS**: Read quickstart.md for integration scenarios

   For scoped execution `T206–T209`, you MUST additionally read:
   - `backend/pkg/corex/db/persistence/vectorstore/pgvector/store.go` (migration expectations / default table & schema)
   - `make_files/database.mk` (the `make db-migrate` entrypoint)
   - `backend/cmd/database/migrate.go` (current migration flow)

4. Parse tasks.md structure and extract:
   - **Task phases**: Setup, Tests, Core, Integration, Polish
   - **Task dependencies**: Sequential vs parallel execution rules
   - **Task details**: ID, description, file paths, parallel markers [P]
   - **Execution flow**: Order and dependency requirements

5. Execute implementation following the task plan:
   - **Phase-by-phase execution**: Complete each phase before moving to the next
   - **Respect dependencies**: Run sequential tasks in order, parallel tasks [P] can run together  
   - **Follow TDD approach**: Execute test tasks before their corresponding implementation tasks
   - **File-based coordination**: Tasks affecting the same files must run sequentially
   - **Validation checkpoints**: Verify each phase completion before proceeding

6. Implementation execution rules:
   - **Setup first**: Initialize project structure, dependencies, configuration
   - **Tests before code**: If you need to write tests for contracts, entities, and integration scenarios
   - **Core development**: Implement models, services, CLI commands, endpoints
   - **Integration work**: Database connections, middleware, logging, external services
   - **Polish and validation**: Unit tests, performance optimization, documentation

7. Progress tracking and error handling:
   - Report progress after each completed task
   - Halt execution if any non-parallel task fails
   - For parallel tasks [P], continue with successful tasks, report failed ones
   - Provide clear error messages with context for debugging
   - Suggest next steps if implementation cannot proceed
   - **IMPORTANT** For completed tasks, make sure to mark the task off as [X] in the tasks file.

8. Completion validation:
   - Verify all required tasks are completed
   - Check that implemented features match the original specification
   - Validate that tests pass and coverage meets requirements
   - Confirm the implementation follows the technical plan
   - Report final status with summary of completed work

## Implementation Guidance (T206–T209)

When implementing `T206–T209`, prefer these patterns:

- Put DDL migrations under `backend/pkg/corex/db/migration/` (idempotent `CREATE ... IF NOT EXISTS`)
- Ensure they are invoked by the same code path that backs `go run ./cmd/database migrate` (used by `make db-migrate`)
- Keep pgvector-specific migrations conditional on `knowledge_space.vector_store.driver == "pgvector"`
- Always create KG assist tables (they are cheap and enable `index.kg` readiness)
- If `CREATE EXTENSION vector` fails due to privilege/availability, return a clear error indicating missing pgvector extension or permissions

For `T209` verification:
- The test must validate both **existence** and **idempotency** (run migrate twice)
- Prefer existing testenv patterns under `backend/tests/**/testenv` when available

Note: This command assumes a complete task breakdown exists in tasks.md. If tasks are incomplete or missing, suggest running `/tasks` first to regenerate the task list.
