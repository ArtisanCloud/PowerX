# Implementation Plan: PowerX Skills 管理与治理

**Branch**: `024-ai-engineering-skills` | **Date**: 2026-03-09 | **Spec**: `/private/var/www/html/ArtisanCloud/X/PowerX/Core/PowerX/specs/024-ai-engineering-skills/spec.md`
**Input**: Feature specification from `/specs/024-ai-engineering-skills/spec.md`

## Summary

交付 PowerX Skills 的平台级治理闭环：官方固有 Skills 目录管理、第三方 Bundle 受控导入、人工审批发布、版本回滚、Capability 绑定，以及 Tenant/Agent 双路径调用一致性。首版明确采用“上传 Bundle + 来源元数据登记、默认最新发布版本调用、checksum 强制校验、signature 策略可配置”的治理策略，并通过多租户隔离与审计链路保障可追溯和可回放。

## Technical Context

**Language/Version**: Go 1.24（backend services），Node 20 + Nuxt 4（web-admin）  
**Primary Dependencies**: Gin HTTP、google.golang.org/grpc（Buf）、GORM、Redis、PostgreSQL、OpenTelemetry、Nuxt UI、Pinia  
**Storage**: PostgreSQL（skills registry + execution trace + audit refs）、Redis（selector/cache/policy snapshot）  
**Testing**: Go `go test`（unit/integration/contract）、OpenAPI/Proto 合约校验、web-admin 端 Vitest/Playwright 冒烟  
**Target Platform**: Linux server + modern browsers  
**Project Type**: CoreX backend module + web-admin management feature  
**Performance Goals**: 合法导入 95% 在 2 分钟内进入 draft；关键调用结果 99% 双路径语义一致  
**Constraints**: 多租户强隔离、发布前 checksum 强制、全量人工审批发布、首版禁止远程仓库在线拉取导入  
**Scale/Scope**: 100+ 租户，单租户 1k+ skill 版本记录，支持 10k+ 日调用审计检索

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

1. **CoreX Boundary (PASS)**：该能力属于 CoreX Agent/Capability 生态治理，不走插件注册机制；实现位于 backend/web-admin 现有 CoreX 结构。  
2. **Dual Transport (PASS)**：规划同时产出 HTTP 合同与 gRPC 合同（specs contracts + backend 对应传输层实现计划），满足规则集 `@dev-crud-http`、`@dev-crud-grpc`。  
3. **Migration & Repository Wiring (PASS)**：数据实体进入 `pkg/corex/db/persistence/model` 与统一迁移入口 `pkg/corex/db/database/migration.go`。  
4. **Tenant Security & Audit (PASS)**：调用前租户与授权检查、调用后审计与 trace 强制记录。  
5. **No Gate Violation (PASS)**：无需要豁免条款。

## Ruleset Alignment (Constitution Gates)

- **HTTP_PRESENT**: 提供 `contracts/http-openapi.yaml`，覆盖 admin/tenant skills 生命周期与调用入口。**PASS**
- **GRPC_PRESENT**: 提供 `contracts/skills-service.proto`，规划 gRPC 服务合同与错误语义映射。**PASS**
- **PROTOBUF_DEFINED**: 计划将 proto 落到 `api/grpc/contracts/powerx/skills/v1` 并通过 buf 生成到 `api/grpc/gen/go`。**PASS**
- **SERVER_DEFINED**: 计划在 `internal/transport/grpc/skills` 实现服务并接入全局拦截器链。**PASS**
- **MAKE_TARGETS**: 复用 `proto-gen/proto-lint/proto-clean`。**PASS**

## Project Structure

### Documentation (this feature)

```
specs/024-ai-engineering-skills/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   ├── http-openapi.yaml
│   └── skills-service.proto
└── tasks.md
```

### Source Code (repository root)

```
backend/
├── api/grpc/contracts/powerx/skills/v1/
├── api/grpc/gen/go/powerx/skills/v1/
├── internal/service/skills/
├── internal/transport/http/admin/skills/
├── internal/transport/http/openapi/skills/
├── internal/transport/grpc/skills/
├── pkg/corex/db/persistence/model/skills/
├── pkg/corex/db/persistence/repository/skills/
└── pkg/corex/db/database/migration.go

web-admin/
├── app/pages/settings/ai/skills.vue
├── app/components/settings/ai/skills/
└── app/composables/api/services/skillsService.ts
```

**Structure Decision**: 采用 CoreX 单体后端 + Nuxt 管理端的既有结构；合同先行（HTTP + gRPC）驱动 service/transport 实现，并由 web-admin 消费 admin contracts。

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| None | - | - |

## Phase 0 – Research Summary

Reference: [`research.md`](./research.md)

- 固化首版导入策略：仅上传 Bundle，来源信息只登记不拉取。
- 固化版本解析：未显式传 version 时默认最新 published。
- 固化发布门禁：全量人工审批，草稿不可直接面向租户。
- 固化完整性策略：checksum 强制，signature 按环境策略可升级为强制。
- 固化官方目录来源：后端内置官方 catalog，随平台版本发布。

## Phase 1 – Design & Contracts

### Data Model (`data-model.md`)

- 定义 `SkillRegistryRecord`、`SkillExecutionTrace`、`SkillCapabilityBinding`、`SkillLifecycleAudit`、`OfficialSkillCatalogEntry`。
- 约束多版本并存、单一最新发布、不可变发布版本、来源与完整性规则。
- 明确状态机和并发冲突策略（发布/回滚互斥）。

### API Contracts (`contracts/`)

- `http-openapi.yaml`：覆盖 admin 列表/导入/发布/回滚/绑定、tenant 直接调用与统一调用。
- `skills-service.proto`：定义 gRPC 管理与调用服务，保证双传输契约一致。

### Quickstart (`quickstart.md`)

- 提供本地启动、迁移、合同校验、导入/发布/调用/回滚的最小闭环步骤。
- 包含审计与指标检查点，确保上线前可复核。

### Agent Context Update

- 将执行 `.specify/scripts/bash/update-agent-context.sh codex`，同步本 feature 新增技术上下文到 Codex 记忆文件。
