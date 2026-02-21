---
# ① Manifest Path（manifest 解析声明）
manifest: .specify/memory/manifest.yaml

# ② 别名启用
use:
  - "@dev-crud-http"
  - "@dev-crud-grpc"
  - "@api-naming"

# ③ 指南文件（用于 /plan 语义扩展）
include:
  - dev_crud_http_guides.md
  - dev_crud_grpc_guides.md
  - dev_sts_guides.md
  - api-naming.md

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
| **CoreX Module** | `internal/{service,dto,transport}/{http,grpc}/<domain>`（示例：`internal/service/iam`、`internal/transport/http/admin/iam`、`internal/transport/grpc/iam`） | **gRPC**：`api/grpc/contracts/powerx/<domain>/v1/*.proto`（权威）；**REST**（OpenAPI）：`specs/<feature>/contracts/http-openapi.yaml`（设计产物） | **HTTP**：`internal/transport/http/(admin|web|openapi)/<domain>`；**gRPC**：`internal/transport/grpc/<domain>` | **不使用** `plugins/registry.json`；由 **CoreX 引导**（`internal/bootstrap/app.go` 及现有装配流程） | **模型/迁移基础**：`pkg/corex/db/persistence/model/...`；迁移注册统一集中在 `pkg/corex/db/database/migration.go` 的 `MigrateCoreModels`（含 `migrate<Domain>Models`）中，由 `cmd/database/migrate.go` 编排调用 |
| **Plugin** | `plugins/<vendor>/<name>/backend/...` | **gRPC**：`api/grpc/<vendor>/<name>/v1/*.proto`；**REST**：`plugins/.../contracts/` | **HTTP**：`plugins/.../transport/http`；**gRPC**：`plugins/.../transport/grpc` | **需要** `plugins/registry.json`；按插件生命周期加载 | `plugins/.../infra/migration` 与插件内 DI/引导 |

> 说明：当前仓库已有约定是 **Proto 的权威源在 `api/grpc/contracts`**，Go 代码生成位置在 `api/grpc/gen/go`（见下文 0.3）。HTTP 的 OpenAPI 合同以 `specs/<feature>/contracts/http-openapi.yaml` 作为设计产物，服务端路由/Handler 以 `internal/transport/http/...` 落地（区分 `admin/web/openapi` 子树）。
> 领域实体说明，因为gorm即定义了model，也可以作为领域的实体使用，不需要反复定义，所以基本上都是在pkg/corex/db/persistence/model/...

- **工具复用（新增）**：凡属通用的转换/JSON/随机/字符串处理等辅助函数，必须集中在 `backend/pkg/utils` 对应模块（如 `xform.go`、`json.go`、`xfind.go` 等），严禁在业务目录重复定义；如遇缺失，应先扩展 utils 模块，再在业务代码中引用。
- **配置文件保护（新增）**：未经用户明确允许，不得修改 `backend/etc/config.yaml`（包括创建、覆盖或清空）。
- **命名规范（新增）**：CoreX 域目录名称一律使用 `snake_case`，以 `capability_registry`、`media_storage` 为例；禁止拼接式命名如 `capabilityregistry`，确保与 Go 包名区分且在跨语言环境保持一致。
- **Go 包别名/调用命名**：引用 `capability_registry` 等多词包时，import alias、局部变量与导出符号统一使用小驼峰（如 `capabilityRegistry`、`capRegPolicy`），避免 `capregpolicy`、`capabilityregistry` 这类连续小写写法。示例：`capabilityRegistry "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/registry"`，通过 `capabilityRegistry.Migrate()`、`capRegPolicy.Register()` 等方式调用以保持可读性。
- **数据访问角色划分（新增）**：`repository` 负责具体持久化实现（GORM/SQL/Redis/MinIO 等），需落在 `pkg/corex/db/persistence/repository/**` 并处理事务/SQL；`interface` 用于 service 层声明所需的数据契约，便于替换实现、注入缓存/内存替身与编写单元测试。Service/handler/任务脚本必须依赖这些接口而非具体 repository，实现切换仅在依赖注入层完成，且 repository 内禁止承载业务逻辑。
- **持久化 Repository 模式**：CoreX 数据访问层统一基于 `pkg/corex/db/persistence/repository/BaseRepository` 泛型封装，具体仓储结构体需嵌入 `BaseRepository[T]` 并显式维护 `db *gorm.DB` 字段，对外暴露以 `New<Xxx>Repository` 命名的构造函数；所有数据访问 API 都以 `ctx context.Context` 与可选 `db *gorm.DB`（事务）为前导参数，业务层不得直接拼接 SQL。
- **数据库迁移注册**：CoreX 模型统一在 `pkg/corex/db/database/migration.go` 的 `MigrateCoreModels`（及其 `migrate<Domain>Models` 子函数）中通过 GORM `AutoMigrate` 注册，`cmd/database/migrate.go` 仅调用该入口。禁止在 `pkg/corex/db/migration/<domain>` 等额外包内自定义入口函数，否则会造成迁移分散与重复。

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
- `COREX_MIGRATION_WIRING`：模型与迁移注册必须遵循“`pkg/corex/db/database/migration.go` 统一挂载 + `cmd/database/migrate.go` 编排”的现有流程，并在 /tasks 明确增量挂载步骤。

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
- **T-COREX-006**：为 `<domain>` 的数据表接入现有 DB 流（模型在 `pkg/corex/db/persistence/model/...`，迁移在 `pkg/corex/db/database/migration.go` 的 `migrate<Domain>Models` 中挂载），并补充回滚策略。  
- **T-COREX-007**：Make 目标：`proto-gen`、`proto-lint`、`proto-clean`、`migrate`、`migrate-down`；CI 校验输出路径/包前缀一致性。
- **T-COREX-008**：契约测试（REST + gRPC）落到 `specs/<feature>/contracts/tests/*.md` → 对应实现下 `_test.go`，严格 TDD。

- ### 0.7 Configuration Assets（新增）

  - **目录分工**：
    - `config/`（仓库根）仅存放跨平台、发布所需的静态资产，如 `config/plugins/`, `config/security/`, `config/version/` 等；这些文件不会被运行时加载。
    - `backend/config/<domain>/...` 是后端运行时配置中心，包含 `platform_capabilities`, `knowledge`, `statebus` 等 YAML。所有 CoreX 运行时配置必须放在该目录或其子目录，禁止在仓库根新建独立 `configs/`.
  - **覆写机制**：各模块可通过环境变量覆写默认路径（如 `PLATFORM_CAPABILITIES_DIR`、`KNOWLEDGE_CONFIG_DIR` 等），但若未配置则均从 `backend/config` 读取。
  - **文档引用**：所有文档/spec/任务在引用运行时配置时必须以 `backend/config/...` 为准，并在需要区分根层 `config/` 资产时明确说明用途。

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
  - `api/grpc/contract/buf.yaml`
  - `api/grpc/contract/buf.gen.yaml` with
    - `managed.go_package_prefix.default = github.com/ArtisanCloud/PowerX/api/grpc/gen`
    - `out = api/grpc/gen`
    - `paths = source_relative`
- **Server (singleton) & Make Targets:**  
  - **Global gRPC bootstrap at `internal/server/grpc/server.go`** with interceptors (`auth`, `tenant`, `logging`, `recovery`)  
  - Make targets: `proto-gen`, `proto-lint`, `proto-clean`
- **Module implementations (no grpc.NewServer, no Register in module):**  
  - `internal/transport/grpc/<module>/*_handler.go`（或 `service.go`）实现生成的 `*ServiceServer` 接口  
  - 通过 `New(*shared.Deps)` 构造，依赖注入 Service；由全局 `server.go` 统一 `Register*ServiceServer(...)`

### X.3 HTTP Handler Response Contract

- 必须使用 `pkg/dto` 中的 `ResponseSuccess`、`ResponseError`、`RespondErrorFrom` 等统一函数输出 JSON；`MustOK` 等旧辅助函数已废弃，禁止继续使用。  
- 所有 Handler 仍遵循“绑定/校验 → 调 Service → 统一回包”的职责分离，错误结构与成功结构必须符合 `pkg/dto/base.go` 中的定义。

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
- Database migrations必须优先使用 GORM AutoMigrate 或等效自动迁移机制；无需单独维护 `up`/`down` 脚本，但需确保迁移过程可重复执行且幂等

### Source Hygiene

- 不得为了“目录占位”而提交仅含包声明或空注释的文件（典型如 `doc.go`、`registry.go`）；新建目录需随首次实现提供实际逻辑、测试或具备实质内容的文档说明。
- 若确需编写包级文档，必须包含有效注释或示例，禁止空壳文件。
- 未落库的辅助结构体（纯内存/DTO/参数）不得携带 `gorm:""` Tag，避免与持久化实体混淆；仅当结构体通过 AutoMigrate 映射至真实表时才允许设置 GORM Tag。
- Service 层必须以结构体方式实现（`*Service` + 构造函数 + 显式依赖注入），禁止额外定义 “业务接口” 壳，以免破坏规则集对集中 DI/事务的约束。
- 依赖注入只负责传递数据库句柄、配置与跨域服务；Repository 由 Service 内部持有并在构造函数中创建，禁止在 `shared.Deps` 层提前实例化 Repo。

### Event Fabric vs. Event Bus

- `pkg/event_bus` 定位为**基础设施层**的发布/订阅抽象（`Publish`、`Subscribe`、`Close`），负责把事件从发布方送到订阅方，不包含主题治理、ACL、重试、死信或回放等业务语义。
- `internal/service/event_fabric/*` 是**领域编排层**，需在 CoreX 事件骨干中完成 Topic 目录、租户 ACL、可靠投递、DLQ、回放、审计等用例，并可组合底层 `pkg/event_bus` 等设施。
- 任何计划/实现不得混淆两者职责：领域服务依赖或扩展基础设施，但禁止在基础设施层堆叠领域逻辑，也不得绕过领域服务直接宣称满足事件骨干需求。
- **实时状态更新强制规范**：Web 管理端涉及任务状态、回放状态、队列执行进度等“实时数据”时，必须走 WebSocket/SSE 推送链路；禁止在页面实现定时轮询（polling）作为主方案。若推送链路不可用，只允许短时人工诊断接口，不得固化为前端常驻轮询逻辑。

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
