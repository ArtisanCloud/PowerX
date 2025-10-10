---
# ① Manifest Path（manifest 解析声明）
manifest: .specify/memory/manifest.yaml

# ② 别名启用
use:
  - "@dev-crud-http"
  - "@dev-crud-grpc"

# ③ 指南文件（用于 /plan 语义扩展）
include:
  - dev_crud_http_guides.md
  - dev_crud_grpc_guides.md
  - dev_sts_guides.md

# ④ Ruleset Paths（显式暴露以便 Runner 能读取）
rulesets:
  - rulesets/crud_http.yaml
  - rulesets/crud_grpc.yaml
  - rulesets/transport_grpc.yaml
  - rulesets/proto_gen.yaml
  - rulesets/crud/migration.yaml
  - rulesets/crud/model.yaml
  - rulesets/crud/repository.yaml
  - rulesets/crud/service.yaml
  - rulesets/crud/dto.yaml
  - rulesets/crud/handler_http.yaml
  - rulesets/crud/api_rest.yaml
  - rulesets/crud/di.yaml
  - rulesets/crud/test.yaml
---

# PowerX Constitution

## ⚙️ Ruleset Declaration

All **planning and tasking phases** (`/plan`, `/tasks`) must load and respect the `manifest.yaml` listed above.  
The runner must resolve every alias in `use:` (`@dev-crud-http`, `@dev-crud-grpc`) against the manifest and merge their `include:` and `rulesets:` definitions into the runtime context before generating any derived documents.

If a runner does not natively support `manifest.yaml`, it must treat this section as a **directive**:  
> “All files under `rulesets/` listed above are authoritative rulesets for HTTP + gRPC CRUD scaffolding and must be enforced in every generated plan or task.”

---

## 🧭 Article 0: CoreX Modules vs. Plugins (Authoritative)

### 0.1 定义与边界

- **CoreX Module（核心组件）**：属于 PowerX 的内核能力域，随 Core 系统一体交付与启动，**不通过插件注册机制**启用。  
  典型域：**IAM、MediaX（媒体存储）、Knowledge（知识库）、Workflow（工作流）、Agent、RBAC/权限管理** 等。  
- **Plugin（外部插件）**：围绕某一功能域的可插拔扩展，遵循插件生命周期与注册机制，通过 `plugins/registry.json` 声明、独立版本化、可按需启停。

> 规划/任务产物必须首先根据 **域声明** 判断归属：若为 CoreX 域 → 走 CoreX 模式；若为外部扩展 → 走 Plugin 模式。禁止将 CoreX 域误生成为插件。

### 0.2 目录与工程结构（强制，与当前仓库对齐）

| 归属 | 源码目录（领域分层） | 合同/接口（权威源） | 传输层实现 | 启动/注册 | 迁移/依赖注入 |
|---|---|---|---|---|---|
| **CoreX Module** | `internal/{service,dto,transport}/{http,grpc}/<domain>`（示例：`internal/service/iam`、`internal/transport/http/admin/iam`、`internal/transport/grpc/iam`） | **gRPC**：`api/grpc/contracts/powerx/<domain>/v1/*.proto`（权威）；**REST**（OpenAPI）：`specs/<feature>/contracts/http-openapi.yaml`（设计产物） | **HTTP**：`internal/transport/http/(admin|web|openapi)/<domain>`；**gRPC**：`internal/transport/grpc/<domain>` | **不使用** `plugins/registry.json`；由 **CoreX 引导**（`internal/bootstrap/app.go` 及现有装配流程） | **模型/迁移基础**：`pkg/corex/db/persistence/model/...`；**特定域表/迁移**按域放置，并在 `cmd/database/migrate.go` 或相应注册处挂载 |
| **Plugin** | `plugins/<vendor>/<name>/backend/...` | **gRPC**：`api/grpc/<vendor>/<name>/v1/*.proto`；**REST**：`plugins/.../contracts/` | **HTTP**：`plugins/.../transport/http`；**gRPC**：`plugins/.../transport/grpc` | **需要** `plugins/registry.json`；按插件生命周期加载 | `plugins/.../infra/migration` 与插件内 DI/引导 |

> 说明：当前仓库已有约定是 **Proto 的权威源在 `api/grpc/contracts`**，Go 代码生成位置在 `api/grpc/gen/go`（见下文 0.3）。HTTP 的 OpenAPI 合同以 `specs/<feature>/contracts/http-openapi.yaml` 作为设计产物，服务端路由/Handler 以 `internal/transport/http/...` 落地（区分 `admin/web/openapi` 子树）。

### 0.3 传输/合同与代码生成（CoreX 统一约束）

- **CoreX 默认双传输**：REST + gRPC（除非该域的 spec 明确豁免）。  
- **gRPC 合同权威源**：`api/grpc/contracts/powerx/<domain>/v1/*.proto`。  
- **Buf 配置（固定位置）**：`api/grpc/contracts/buf.yaml`、`api/grpc/contracts/buf.gen.yaml`。  
- **Go 代码生成输出**：`api/grpc/gen/go`（保持 `paths = source_relative`）；包前缀与路径需与 `powerx/<domain>/v1` 对齐。  
- **gRPC 服务端实现**：`internal/transport/grpc/<domain>`；**拦截器链**必须包含：`auth`、`tenant`、`logging`、`recovery`。  
- **HTTP 服务端实现**：`internal/transport/http/(admin|web|openapi)/<domain>`；路由聚合仍由 `internal/http/router.go`（以及各 api.go）统一挂载。  
- **Make 目标**：
  - proto-gen：使用 buf 按 buf.gen.yaml 中的插件配置生成代
  - proto-lint：使用 buf 对 .proto 文件进行规范检查
  - proto-clean：清理由生成产物目录（如 gen/）
（可选）proto-fmt：使用 buf 自动格式化 .proto 文件
- **设计/实现的单一事实来源（SoT）**：  
  - Proto：以 `api/grpc/contracts/...` 为 SoT，禁止在实现侧重复维护“第二份 proto”。  
  - OpenAPI：以 `specs/<feature>/contracts/http-openapi.yaml` 为 SoT，服务端以该合同为准落地并更新测试。

### 0.4 /plan 与 /tasks 的 Gate（CoreX 阻断性条款）

当且仅当该域声明为 CoreX Module 时，/plan 与 /tasks 必须满足：

- `COREX_DECLARED`：在 plan 的结构/路径小节明确此域为 **CoreX Module**（非插件）。  
- `NO_PLUGIN_REGISTRY`：**不得**包含 `plugins/registry.json` 注册或 `plugins/...` 目录步骤。  
- `COREX_LAYOUT_MATCH`：路径必须符合 0.2 表（`internal/service/<domain>`、`internal/transport/{http,grpc}/<domain>` 等）。  
- `COREX_DUAL_TRANSPORT`：除非 spec 明确豁免，需同时给出 **REST** 与 **gRPC** 的合同与实现规划。  
- `COREX_BUF_CONFIG`：`api/grpc/contracts/buf.yaml` 与 `buf.gen.yaml` 存在且配置正确；生成输出到 `api/grpc/gen/go`。  
- `COREX_SERVER_WIRING`：gRPC/HTTP 装配通过现有 CoreX 引导：`internal/bootstrap/app.go`、`internal/http/router.go`（以及各 `api.go` 集中导出）。  
- `COREX_MIGRATION_WIRING`：模型与迁移注册与现有 `pkg/corex/db/...` 及 `cmd/database/migrate.go` 流程一致，并在 /tasks 明确增量挂载步骤。

### 0.5 CoreX 域声明与扩展（可逐步追加）

- 当前 CoreX 域（白名单）：`corex.iam`、`corex.mediax`、`corex.knowledge`、`corex.workflow`、`corex.agent`、`corex.rbac`。  
- 新增 CoreX 域时：  
  1) 在该 feature 的 `spec.md` 顶部添加注记：**Domain Ownership: CoreX (`corex.<domain>`)**；  
  2) `/plan` 的 “Project Structure” 小节必须输出与 0.2 对齐的 **CoreX 路径与装配**；  
  3) `/tasks` 生成对应的 **COREX Gate** 任务（见 0.6）。

### 0.6 /tasks 任务模板（CoreX 版，与你现有工程对齐）

- **T-COREX-001**：为 `<domain>` 创建/补齐目录骨架：  
  `internal/service/<domain>/`、`internal/dto/<domain>/`、`internal/transport/http/(admin|web|openapi)/<domain>/`、`internal/transport/grpc/<domain>/`。  
- **T-COREX-002**：在 `api/grpc/contracts/powerx/<domain>/v1/` 定义 proto；维护 `api/grpc/contracts/{buf.yaml,buf.gen.yaml}`；生成到 `api/grpc/gen/go`。  
- **T-COREX-003**：实现 `internal/transport/grpc/<domain>`（拦截器链 `auth/tenant/logging/recovery`）。  
- **T-COREX-004**：实现 `internal/transport/http/(admin|web|openapi)/<domain>` 与路由装配；合同以 `specs/<feature>/contracts/http-openapi.yaml` 为 SoT。  
- **T-COREX-005**：在 `internal/bootstrap/app.go`、`internal/http/router.go`（以及 `<domain>/api.go`）挂载 HTTP/gRPC。  
- **T-COREX-006**：为 `<domain>` 的数据表/迁移脚本接入现有 DB 流（模型在 `pkg/corex/db/persistence/model/...`，迁移流程在 `cmd/database/migrate.go`）；补充回滚策略。  
- **T-COREX-007**：Make 目标：`proto-gen`、`proto-lint`、`proto-clean`、`migrate`、`migrate-down`；CI 校验输出路径/包前缀一致性。  
- **T-COREX-008**：契约测试（REST + gRPC）落到 `specs/<feature>/contracts/tests/*.md` → 对应实现下 `_test.go`，严格 TDD。

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

## 🧩 Article X: Transport Mandate & Ruleset Enforcement

### X.1 Dual-Transport Requirement

Every plugin **MUST** expose both **HTTP/REST** and **gRPC** transports:

| Transport | Required Rulesets | Expected Outputs |
|------------|------------------|------------------|
| HTTP / REST | `crud_http.yaml`, `handler_http.yaml`, `api_rest.yaml`, `dto.yaml`, `service.yaml`, `repository.yaml` | REST handlers, DTOs, API routes |
| gRPC | `crud_grpc.yaml`, `transport_grpc.yaml`, `proto_gen.yaml` | Protobuf schemas, buf.yaml, buf.gen.yaml, gRPC server with interceptors |

### X.2 Generation Artifacts (gRPC)

- **Protobuf** files at `api/grpc/<domain>/v1/*.proto`
- **Buf** configs:
    - `buf.yaml`
    - `buf.gen.yaml` with
        - `managed.go_package_prefix.default = github.com/ArtisanCloud/PowerX/internal/transport/grpc/gen`
        - `out = internal/transport/grpc/gen`
        - `paths = source_relative`
- **Server (singleton) & Make Targets:**
    - **Global gRPC bootstrap at `internal/server/grpc/server.go`** with interceptors (`auth`, `tenant`, `logging`, `recovery`)
    - Make targets: `proto-gen`, `proto-lint`, `proto-clean`
- **Module implementations (no grpc.NewServer, no Register in module):**
    - `internal/transport/grpc/<module>/*_handler.go`（或 `service.go`）实现生成的 `*ServiceServer` 接口
    - 通过 `New(*shared.Deps)` 构造，依赖注入 Service；由全局 `server.go` 统一 `Register*ServiceServer(...)`

### X.3 Blocking Gates

The following gates are **non-negotiable** and must be validated in `/plan` and `/tasks` outputs:

| Gate | Description |
|------|--------------|
| `HTTP_PRESENT` | Must include Ruleset Alignment（@dev-crud-http）section |
| `GRPC_PRESENT` | Must include Ruleset Alignment（@dev-crud-grpc）section |
| `PROTOBUF_DEFINED` | buf.yaml + buf.gen.yaml present with go_package_prefix |
| `SERVER_DEFINED` | gRPC server implementation + interceptors declared |
| `MAKE_TARGETS` | proto-gen / proto-lint / proto-clean defined |

Any plan missing the above gates is **invalid** and fails constitutional compliance.

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

**Version**: 2.0.1 | **Ratified**: 2025-10-10 | **Last Amended**: 2025-10-10
