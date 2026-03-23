# Implementation Plan: PowerX Skills 管理与治理

**Branch**: `024-ai-engineering-skills` | **Date**: 2026-03-09 | **Spec**: `/private/var/www/html/ArtisanCloud/X/PowerX/Core/PowerX/specs/024-ai-engineering-skills/spec.md`
**Input**: Feature specification from `/specs/024-ai-engineering-skills/spec.md`

## Summary

交付 PowerX Skills 的平台级治理与 Agent 统一编排闭环：官方固有 Skills 目录管理、第三方 Bundle 受控导入、人工审批发布、版本回滚、Capability 绑定、Tenant/Agent 双路径调用一致性，以及“LLM 统一意图识别 + workflow/skill/tooling/llm 计划编排（串并行） + LLM Tool-Calling”统一执行。当前基线采用“上传 Bundle + 来源元数据登记、默认最新发布版本调用、checksum 强制校验、signature 策略可配置”，并通过多租户隔离与审计链路保障可追溯和可回放。
新增对齐目标：建立 Provider 无关的 Context 优化机制（分层上下文、预算裁剪、结构化摘要、Prompt/Context Cache 与 token 观测），在不改变 LLM 主路由原则下显著降低 token 成本与时延。

## Technical Context

**Language/Version**: Go 1.24（backend services），Node 20 + Nuxt 4（web-admin）  
**Primary Dependencies**: Gin HTTP、google.golang.org/grpc（Buf）、GORM、Redis、PostgreSQL、OpenTelemetry、Nuxt UI、Pinia  
**Storage**: PostgreSQL（skills registry/execution trace/audit refs + capability registry tooling catalog/trace）、Redis（selector/cache/policy snapshot）  
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

## Phase 2 – Runtime Baseline (Unified Planner + Tool-Calling)

1. **入口收敛**：以 `/api/v1/agents/invoke` 与 `/api/v1/agents/stream/sse` 为主入口承载完整闭环；tenant `/tenant/skills/invoke` 与 `/tenant/invocations` 保留为执行层接口。  
2. **统一候选池**：意图阶段输出 `workflow|skill|tooling` 多候选（top-k），并附 `score/reason/constraints`。  
3. **计划编排**：Planner 产出 DAG/阶段计划，节点包含 `workflow|skill|tooling|llm`，支持串行依赖、并行批次、失败策略。  
4. **LLM Tool-Calling**：在受控候选清单内进行 tool 选择，选择结果回落到 registry/binding 的硬过滤约束。  
5. **无命中回落**：当未命中可执行节点时，直接进入 `llm` 回复，不得强行匹配 Flow。  
6. **观测升级**：trace 从 `trace_id` 扩展到 `plan_id + node_id` 维度，审计可还原计划执行路径。
7. **执行器升级**：`node.kind=skill|tooling` 必须接真实服务调用链路，禁止占位返回；其中 tooling 以 capability registry 数据库为权威源。
8. **候选分层**：候选池按 `workflow|skill|tooling` 分区，并按 `system builtin + agent custom` 双层来源合并去重后再进入 LLM 决策。
9. **提示词分区**：LLM 决策输入采用结构化分区清单（workflow/skill/tooling + source 标记），禁止仅用未分区的长文本拼接。

## Phase 12 – Context Optimization Baseline

Reference: [`context-optimization.md`](./context-optimization.md)

1. **上下文分层**：统一实现 `L0-L5` 分层拼装链路（固定前缀 → 能力目录摘要 → 会话摘要 → 最近窗口 → 检索片段 → 当前输入），并确保稳定顺序。  
2. **预算管理**：引入请求前 token 预算器，超预算时执行“检索裁剪 → 最近窗口裁剪 → 摘要刷新”的降载策略。  
3. **结构化记忆**：将会话摘要升级为结构化 schema（facts/decisions/open_issues/constraints），替代纯文本占位摘要。  
4. **缓存策略**：引入 `cache_mode=auto|force_off|force_on`，按 provider/model 能力探测决定 Prompt/Context Cache 启用。  
5. **可观测性**：对每次调用补齐 `prompt_tokens/completion_tokens/cached_tokens/context_layers_size/trim_actions`，并落盘到 debug trace 与审计聚合字段。  
6. **灰度发布**：按“仅观测 → 软裁剪 → 默认启用”三阶段推进，保留租户级回滚开关。

## Implementation Backwrite (2026-03-19)

- 已完成 `T046`：新增 `backend/internal/service/skills/lifecycle_integrity_invoke_test.go`，覆盖状态机、完整性策略、默认版本解析单元测试。
- 已完成 `T049`：新增 `backend/tests/integration/skills/skill_nonfunc_integration_test.go`，覆盖导入耗时、调用一致性、审计写入基线。
- 运维注意事项：当前 `SkillExecutionTrace` 以 `trace_id` 唯一键存储，单次调用“resolved + completed”双写会发生后写冲突。现阶段基线统计以 `resolved` 事件为准，后续建议将 trace 写入改为 Upsert 或拆分事件模型。
- 已完成 `T071-T075`：Agent `skill/tooling` 节点已从占位执行升级为真实调用（skill invoke/adaptor + capability invocation），并补齐 `tenant_uuid/user_id/env/trace_id` 透传；文档已同步 tooling 落库权威说明。
- 已完成 `T076-T081`：候选池支持 `system builtin + agent custom` 双层聚合与同名去重（agent 优先），统一硬过滤在候选构建阶段前置执行；组合规划可串联 `workflow->skill/tooling`，并在 `plan/tasks` 与 SSE 节点事件中透传 `node_kind/node_ref/source_scope`。
- 测试矩阵已补齐：新增 `skill_agent_candidate_layering_test.go`（双层候选去重 + 未授权不可见）与 `skill_agent_composite_plan_test.go`（组合规划执行 + 事件字段校验），并纳入运行时回归清单。
