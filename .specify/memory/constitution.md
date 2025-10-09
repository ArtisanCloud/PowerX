---
use:
  - "@dev-crud-http"
---

# PowerX Constitution

## Core Principles

### I. Plugin-First Architecture

Every functional domain in PowerX is delivered as a **plugin**.  
Plugins must be self-contained, independently testable, and versioned.  
No business logic is allowed inside the Core kernel.  
Each plugin declares its own capabilities and contracts (`provides` / `consumes`) and interacts only via official interfaces (gRPC, Event Bus, Contract SDK).

### II. Spec-Driven Development

All work begins from a specification (`spec.md`), not code.  
Each feature follows the full Spec-Kit lifecycle:  
`/specify → /clarify → /plan → /tasks → /implement → /analyze`.  
Every feature lives under `specs/<domain>/<feature>/` and must include the **Spec Triplet**:  
`spec.md`, `plan.md`, and `tasks.md`.  
No implementation without an approved spec.

### III. Multi-Tenant & Secure-by-Design

PowerX is built for secure, isolated multi-tenant operation.  
All APIs, storage layers, and cache/queue systems must include tenant context.  
RBAC authorization and audit trails are mandatory.  
Every plugin operates in a scoped sandbox with least privilege.

### IV. Agent & Workflow Integration

PowerX includes an Agent Runtime and Workflow Engine.  
All workflows are declarative YAML or spec-based, and all agent actions are traceable and auditable.  
Agents may invoke plugins, but not modify their schemas or binaries.  
Each Agent or MCP must comply with this Constitution when executing automated commands.

### V. Observability & Quality Gates

Every service and plugin must provide:

- Structured JSON logging with `trace_id` and `tenant_id`
- Metrics (`qps`, `error_rate`, `p95_latency`)
- OpenTelemetry tracing across all calls
- 80% minimum test coverage and performance baselines  
No merge or release is allowed without passing quality gates.

---

## Additional Constraints

### Security & Compliance

- Authentication: JWT/OIDC with asymmetric keys only (RS256/JWKS).  
- Authorization: Unified RBAC model (`<domain.action>`).  
- Data Protection: Encrypted at rest & in transit.  
- Dependency scanning (`make deps-audit`) weekly; Critical CVEs patched within 24h.

### Performance Standards

- API p95 latency < 200ms  
- Plugin startup < 5s  
- Plugin memory < 256MB (unless justified)  
- Database migrations must include both `up` and `down` scripts

---

## Development Workflow

1. **Specification First** — every feature begins with `/specify`  
2. **Clarification Round** — unresolved ambiguities handled via `/clarify`  
3. **Planning & Constitution Check** — `/plan` validates design against this Constitution  
4. **Task Generation** — `/tasks` outputs TDD-style task list  
5. **Implementation** — `/implement` executes with tests-first principle  
6. **Post-Review** — `/analyze` enforces consistency, coverage, and metrics alignment

Code reviews must verify:

- Spec Triplet completeness  
- RBAC, audit, and tenant isolation  
- Metrics + Trace integration  
- Passing of Constitution checks

---

## Governance

This Constitution supersedes all other conventions.  
Amendments require:

- Documented RFC with motivation, impact, and migration plan  
- Approval by the **PowerX Technical Council**  
- Version bump and announcement via Spec-Kit sync  
All PRs must verify Constitution compliance and complexity justification.

**Version**: 2.0.0 | **Ratified**: 2025-10-07 | **Last Amended**: 2025-10-07
