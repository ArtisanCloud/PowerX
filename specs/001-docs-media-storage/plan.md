
# Implementation Plan: Media Asset Admin Capabilities

**Branch**: `001-docs-media-storage` | **Date**: 2025-10-07 | **Spec**: [/private/var/www/html/ArtisanCloud/X/PowerX/Core/PowerX/specs/001-docs-media-storage/spec.md](spec.md)
**Input**: Feature specification from `/specs/001-docs-media-storage/spec.md`

## Execution Flow (/plan command scope)

```
1. Load feature spec from Input path
   → If not found: ERROR "No feature spec at {path}"
2. Fill Technical Context (scan for NEEDS CLARIFICATION)
   → Detect Project Type from file system structure or context (web=frontend+backend, mobile=app+api)
   → Set Structure Decision based on project type
3. Fill the Constitution Check section based on the content of the constitution document.
4. Evaluate Constitution Check section below
   → If violations exist: Document in Complexity Tracking
   → If no justification possible: ERROR "Simplify approach first"
   → Update Progress Tracking: Initial Constitution Check
5. Execute Phase 0 → research.md
   → If NEEDS CLARIFICATION remain: ERROR "Resolve unknowns"
6. Execute Phase 1 → contracts, data-model.md, quickstart.md, agent-specific template file (e.g., `CLAUDE.md` for Claude Code, `.github/copilot-instructions.md` for GitHub Copilot, `GEMINI.md` for Gemini CLI, `QWEN.md` for Qwen Code, or `AGENTS.md` for all other agents).
7. Re-evaluate Constitution Check section
   → If new violations: Refactor design, return to Phase 1
   → Update Progress Tracking: Post-Design Constitution Check
8. Plan Phase 2 → Describe task generation approach (DO NOT create tasks.md)
9. STOP - Ready for /tasks command
```

**IMPORTANT**: The /plan command STOPS at step 7. Phases 2-4 are executed by other commands:

- Phase 2: /tasks command creates tasks.md
- Phase 3-4: Implementation execution (manual or via tools)

## Summary

构建后台媒体资产管理能力：支持上传（直传或外链）、分页检索、详情、业务字段更新、软删除 + 定时物理清理，以及 12 小时默认有效的预签名链接。方案沿用现有 `handler → service → repository → model` 分层，引入 `internal/infra/media` 的多驱动 `MediaManager`，并扩展 `pkg/corex/db/persistence` 下的媒体模型与仓储，确保 RBAC、审计和多租户上下文贯通。

## Technical Context

**Language/Version**: Go 1.24  
**Primary Dependencies**: Gin HTTP 框架、GORM、PowerX 内部 `dto`/`repository` 基础库、计划新增的 MinIO S3 SDK  
**Storage**: MySQL（经由 GORM）；对象存储（S3 兼容 & 本地文件系统）  
**Testing**: Go 原生 testing + httptest；`make unit-test` (`go test ./...`)  
**Target Platform**: Linux 容器化服务（PowerX 后端）  
**Project Type**: backend-service（单体 Go 项目，按 internal/pkg 分层）  
**Performance Goals**: API p95 < 200ms；预签名生成 < 100ms；列表分页稳定在 100ms 内  
**Constraints**: 必须携带租户与操作人上下文；严格 RBAC；全链路 JSON 日志/trace；软删后由定时任务清理物理对象  
**Scale/Scope**: 预期每租户 5~10 万媒体记录，单次上传 <10 并发；标签/筛选高选择性

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **Plugin-First Architecture**：媒体功能仍位于核心服务的模块化目录下（internal/service、internal/infra、pkg/corex），不侵入核心内核，满足插件化分层。
- **Spec-Driven Development**：`spec.md` 与 `clarifications` 已完成，Plan 基于最新规格执行。
- **Multi-Tenant & Secure-by-Design**：规划中所有接口沿用后台授权、租户上下文与操作审计；软删与预签名均需鉴权。
- **Agent & Workflow Integration**：MediaManager 作为内部基础设施组件，可由 Agent 调度但不越权；无违规。
- **Observability & Quality Gates**：方案要求写入统一日志（trace_id/tenant_id）、指标与审计；测试覆盖将通过 tasks 阶段落地。
- **Post-Design Review**：Phase 1 设计落地后未新增核心侵入或安全风险，维持合规。
**结论**：未发现违宪点，进入 Phase 0 研究。

## Project Structure

### Documentation (this feature)

```
specs/001-docs-media-storage/
├── plan.md              # This file (/plan command output)
├── research.md          # Phase 0 output (/plan command)
├── data-model.md        # Phase 1 output (/plan command)
├── quickstart.md        # Phase 1 output (/plan command)
├── contracts/           # Phase 1 output (/plan command)
└── tasks.md             # Phase 2 output (/tasks command - NOT created by /plan)
```

### Source Code (repository root)
<!--
  ACTION REQUIRED: Replace the placeholder tree below with the concrete layout
  for this feature. Delete unused options and expand the chosen structure with
  real paths (e.g., apps/admin, packages/something). The delivered plan must
  not include Option labels.
-->
```
cmd/
├── agent/
├── demo/
└── test_agent_api/

config/
└── storage.go (待新增媒体驱动配置)

internal/
├── bootstrap/
├── app/
│   └── shared/
├── infra/
│   └── media/
│       ├── driver/
│       │   ├── local/
│       │   └── s3/
│       └── manager/
├── service/
│   └── media/
└── transport/
    └── http/
        └── admin/
            └── media/

pkg/
└── corex/
    └── db/
        └── persistence/
            ├── model/
            │   └── media/
            └── repository/
                └── media/
```

**Structure Decision**: 继续采用单体 Go backend 布局；新增媒体相关代码分别进入 `internal/transport/http/admin/media`、`internal/service/media`、`internal/infra/media` 与 `pkg/corex/db/persistence/{model,repository}/media`，确保与现有分层一致。

## Phase 0: Outline & Research

1. **Extract unknowns from Technical Context** above:
   - 明确 MediaAsset 的唯一性约束（UUID 外是否需要 driver+objectKey 唯一索引）。
   - 评估每租户 10 万级资产对分页与查询索引的影响，确认搜索策略与缓存需求。
   - 选型 MinIO Go SDK（或其他 S3 客户端）的最佳实践：重试、分片上传、凭证管理。
   - 调研标签字段 (JSON) 的检索需求与潜在索引策略。
   - 梳理软删后定时物理清理的运维模式：任务频率、失败重试、审计记录。

2. **Generate and dispatch research agents**:

   ```
   - Research MediaAsset uniqueness rules and indexing for multi-tenant media storage (PowerX).
   - Research pagination/search strategy for ~100k records per tenant with optional tag filters using GORM/MySQL.
   - Research go-minio best practices for backend-admin uploads (timeouts, retries, presign, IAM security).
   - Research efficient storage and querying patterns for JSON tag arrays in MySQL (with GORM).
   - Research operational playbook for scheduled cleanup of soft-deleted objects (observability + alerting).
   ```

3. **Consolidate findings** in `research.md` using format:
   - Decision: [what was chosen]
   - Rationale: [why chosen]
   - Alternatives considered: [what else evaluated]

**Output**: research.md with all NEEDS CLARIFICATION resolved

## Phase 1: Design & Contracts

*Prerequisites: research.md complete*

1. **Extract entities from feature spec** → `data-model.md`:
   - Entity name, fields, relationships
   - Validation rules from requirements
   - State transitions if applicable

2. **Generate API contracts** from functional requirements:
   - Upload、List、Get、Update、Delete、GeneratePresign 对应 REST 端点。
   - 描述请求/响应字段（分页结构、业务状态枚举、审计字段）。
   - 输出 OpenAPI 片段到 `contracts/admin-media-assets.yaml`，同时提供 JSON 示例。

3. **Generate contract tests** from contracts:
   - `contracts/tests/admin_media_assets_test.go` 基于 httptest，验证路由绑定和 JSON Schema（占位失败）。
   - 使用 Table-Driven 框架覆盖成功/失败场景占位。

4. **Extract test scenarios** from user stories:
   - Primary story → Quickstart 步骤（上传→检索→详情→生成预签名）。
   - 第二条验收场景衍生列表筛选集成测试。

5. **Update agent file incrementally** (O(1) operation):
   - Run `.specify/scripts/bash/update-agent-context.sh codex`
     **IMPORTANT**: Execute it exactly as specified above. Do not add or remove any arguments.
   - If exists: Add only NEW tech from current plan
   - Preserve manual additions between markers
   - Update recent changes (keep last 3)
   - Keep under 150 lines for token efficiency
   - Output to repository root

**Output**: data-model.md, /contracts/*, failing tests, quickstart.md, agent-specific file

## Phase 2: Task Planning Approach

*This section describes what the /tasks command will do - DO NOT execute during /plan*

**Task Generation Strategy**:

- Load `.specify/templates/tasks-template.md` as base
- Generate tasks from Phase 1 design docs (contracts, data model, quickstart)
- Each contract → contract test task [P]
- Each entity → model creation task [P]
- Each user story → integration test task
- Implementation tasks to make tests pass

**Ordering Strategy**:

- TDD order: Tests before implementation
- Dependency order: Models before services before UI
- Mark [P] for parallel execution (independent files)

**Estimated Output**: 25-30 numbered, ordered tasks in tasks.md

**IMPORTANT**: This phase is executed by the /tasks command, NOT by /plan

## Phase 3+: Future Implementation

*These phases are beyond the scope of the /plan command*

**Phase 3**: Task execution (/tasks command creates tasks.md)  
**Phase 4**: Implementation (execute tasks.md following constitutional principles)  
**Phase 5**: Validation (run tests, execute quickstart.md, performance validation)

## Complexity Tracking

*Fill ONLY if Constitution Check has violations that must be justified*

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| *(None)* | — | — |

## Progress Tracking

*This checklist is updated during execution flow*

**Phase Status**:

- [x] Phase 0: Research complete (/plan command)
- [x] Phase 1: Design complete (/plan command)
- [x] Phase 2: Task planning complete (/plan command - describe approach only)
- [ ] Phase 3: Tasks generated (/tasks command)
- [ ] Phase 4: Implementation complete
- [ ] Phase 5: Validation passed

**Gate Status**:

- [x] Initial Constitution Check: PASS
- [x] Post-Design Constitution Check: PASS
- [x] All NEEDS CLARIFICATION resolved
- [x] Complexity deviations documented

---
*Based on Constitution v2.1.1 - See `/memory/constitution.md`*
