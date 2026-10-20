# T198：工具、Capability 与 Skill executor 边界审计

审计日期：2026-10-13。范围为当前 Core 工作区的生产装配、调用链及本地测试；未修改后端代码、数据库、发布版本或运行服务。本报告完成 T198 的清点与违规定位，不代表 T199～T205 的生产接线、整改或真实业务验收完成。

约束来源：[024 spec FR-079～FR-086](./spec.md)、[分层开发规范](../../docs/guides/develop/agent-skill-tool-boundaries.md)、[V4 实现说明](../../docs/guides/develop/agent-response-evidence-v4.md)。

## 1. 已确认的生产调用链

### 1.1 Agent Skill 主执行链

```text
Agent Manager 的 SkillInvoker
→ DefinitionInvokeService.Execute
→ 本租户的已发布 Draft / Current Revision
→ 校验发布产物、checksum、请求 revision 和 entrypoint
→ ManifestExecutor.Execute
→ CheckToolDependencies
→ executor.type 对应的生产端口
```

装配见 [bootstrap/init.go](../../backend/internal/server/agent/bootstrap/init.go)，定义读取见 [definition_invoke_service.go](../../backend/internal/service/skills/definition_invoke_service.go)。执行器本身依据 `executor.type` 选择，不依据 Skill ID 选择；读取定义或依赖失败不会回退到 Registry、本地文件或模型心算。

这条链存在，不代表其它 transport 已迁移到同一条链，也不代表 Runtime 的任务选择层已清除业务分支。

### 1.2 V4 内置计算链

```text
llm_prompt + output_mode=response_envelope
→ 原文 token / facts
→ 已发布 calculation_policy 的字段映射与公式计划
→ evidence.ExecutePlan / Calculate
→ 真实计算记录与同次执行 ledger
→ notes / review_policy
→ V4 报告
→ Runtime ValidateReport + evidence.Verify
```

这条链可作为内置纯计算与报告分类的复用基础。业务字段、单位、公式和说明策略来自 Skill Revision；同次执行 ledger 验证报告由本次运行产生，不授予跨租户数据访问权限，也不证明用户材料或业务归因真实。

## 2. 复用能力与边界清单

| 对象 | 当前实际能力 | 接入与限制 |
|---|---|---|
| `DefinitionInvokeService` | 读取租户已发布 Revision，校验固定 revision、产物与 checksum，再调用 executor。 | 可复用为治理态执行入口；旧 Registry transport 尚未统一接入。 |
| `ManifestExecutor` | 声明 `llm_prompt`、`capability`、`workflow`、`instruction_only` 四种类型；非法类型显式拒绝。 | 类型可被解析不等于端口可执行。生产 Workflow 端口仍明确返回 unavailable；没有受控脚本 executor。 |
| LLM 生产端口 | 经 `ChatConfigResolver` 和唯一 AI Service 调用模型，读取 Skill model_policy、请求 locale 和 Schema。 | 需要租户与 Agent 上下文；缺模型、locale、空结果或阶段校验错误显式失败。 |
| Capability 生产端口 | 经 Agent 授权服务再进入 Capability Invocation，当前请求固定 `PreferredProtocol=core_internal`。 | 不包含 `ToolDependencyChecker` 接线或外部工具精确版本回执；不能据此声称所有插件协议均可用于 Skill 依赖。 |
| Capability Invocation adapters | 当前调用链支持已接入的 Core invoker、HTTP/REST；gRPC 需要额外连接配置，缺配置明确失败。 | Agent 装配未提供 gRPC conn；该 adapter switch 没有 MCP 分支。协议元数据存在不能证明生产端口已接线。 |
| 内置计算 `powerx.calculation.evaluate@1.0.0` | 受限算术 AST、有理数、显式精度与比例、确定性舍入；资源限制、取消和除零拒绝。 | 不执行任意代码或网络；不能提供领域算法、数据真实性或业务归因判断。报告链额外禁止模型用数字常量伪造业务操作数。 |
| `CheckToolDependencies` | 校验 `powerx.skill-tools/v1`、重复 key、字段完整性、内置计算精确版本与 Schema。外部依赖无 checker 时拒绝。 | 发布、绑定和执行均有调用点，但生产构造器没有传入外部 checker；测试回调通过不是生产授权证明。 |
| V4 token / ledger / report 校验 | 来源定位、原数字与单位保留、报告值/计算值/冲突分离、同次 ledger 验真、修改或跨运行重用拒绝。 | computed 校验目前限定内置计算 key/version；未接外部工具回执与通用受授权对象引用解析。 |
| 插件 Skill Discovery / Registry Sync | 校验插件 manifest，写入 Registry 的来源、版本和 capability binding 元数据。 | 没有据此生成可执行 Definition Revision 的生产闭环；`published` Registry 记录不等于已通过新版可运行检查。Discovery 地址仍固定 `/api/v1/plugin/skills`，需按前缀规范独立核对。 |
| Skill 包安装与导入 | 可读取 `SKILL.md` 并登记 metadata；安装流程存在 Python/git 子进程。 | 安装子进程用于获取包，不是导入脚本的 Runtime executor；不能把安装成功解释为脚本获执行权限。 |

主要源码：

- [executor_manifest.go](../../backend/internal/service/skills/executor_manifest.go)
- [tool_dependencies.go](../../backend/internal/service/skills/tool_dependencies.go)
- [definition_service.go](../../backend/internal/service/skills/definition_service.go)、[definition_readiness.go](../../backend/internal/service/skills/definition_readiness.go)
- [calculator.go](../../backend/pkg/corex/agent/evidence/calculator.go)、[report.go](../../backend/pkg/corex/agent/evidence/report.go)、[schema.go](../../backend/pkg/corex/agent/evidence/schema.go)
- [Capability Invocation](../../backend/internal/service/capability_registry/invocation_service.go)、[adapter 选择](../../backend/internal/service/capability_registry/router/service.go)
- [plugin_bridge.go](../../backend/internal/service/skills/plugin_bridge.go)、[plugin_registry_handler.go](../../backend/internal/transport/http/admin/skills/plugin_registry_handler.go)

## 3. 已确认缺口与违规点

### P23-B1：Runtime 按营销 Skill ID 分派业务任务

[marketing_review_route.go](../../backend/internal/server/agent/runtime/marketing_review_route.go) 固定识别 `marketing.review_summarize`，并在 Go 中维护营销/活动/复盘/ROI 等关键词、任务 ID、策略与原因。`Engine.detectTasks()` 在调用通用 Tool-Calling 之前先执行该分支，见 [engine.go](../../backend/internal/server/agent/runtime/engine.go)。

这条分支仍检查当前绑定与候选集，所以不能把它描述为已证明绕过授权；但任务选择依赖业务 Skill ID 和业务关键词，明确违反 FR-079 的通用 Runtime 边界。改名或使用相同合同的客户自建 Skill 不会得到同样路径。

现有 `marketing_review_route_test.go` 的三项测试通过，证明该分支存在并被测试固定；测试通过不能证明架构合规。种子中维护营销 Skill 数据、通用代码按 UUID 查找绑定以及 capability dispatcher 根据正式能力选择领域工具，不能与这个业务路由分支混为一谈。

整改要求：业务触发语义进入版本化、已发布的 Skill/候选配置，由通用选择机制消费；不能另加业务 key 白名单。T199/T203/T204 必须明确合同与迁移，并用改名、自建非营销 Skill 和正常候选授权测试验证。

### P23-B2：外部工具依赖缺少生产 checker 与回执合同

`ManifestExecutorOptions.CheckToolDependencies` 和 `NewDefinitionService(...checkers)` 已定义扩展端口。生产 [newDefinitionManifestExecutor](../../backend/internal/server/agent/bootstrap/init.go) 没有注入 checker；[Agent 绑定检查](../../backend/internal/service/agent/agent_service.go) 和原生 Skill seed 构造 DefinitionService 时同样未注入。

因此外部依赖目前会以 `skill.tool_dependency_unavailable` 阻断，属于失败关闭，不能宣称外部工具已接入。`ToolDependency` 当前只有字符串 key/version/input_schema/output_schema/permission，computed 报告仅接受内置计算工具。还缺可解析的正式能力身份、实际固定版本、发布状态、调用主体/租户授权、Schema 定位及可信结果回执的统一合同。

归属：T199 定义合同，T200 接入发布/绑定/执行检查，T202 核验实际来源与结果，T204/T205 验证客户自建 Skill 和真实插件工具。若新增正式 Core capability，须遵守 Admin/service 双授权面与 capability-check/seed/grant-status 要求；本轮不新增或变更能力。

### P23-B3：Workflow / 脚本不能按声明类型视为可运行

生产 Manifest Workflow 端口直接返回 `skill.executor_workflow_unavailable`。`instruction_only` 明确不可运行；不支持的脚本类型无法通过 Definition executor 校验。安装器的 Python/git 调用不构成受控脚本执行环境。

后续发布与可运行绑定检查应区分“合同类型合法”和“当前部署 executor 可用”，继续保留非可执行 Draft。Workflow 与脚本需要真实通用接线及隔离/资源/结果合同，不能用占位返回标记 completed。

### P23-B4：旧 transport 仍使用未注入 executor 的 Registry InvokeService

当前四个生产入口直接构造 `NewInvokeService`，没有 `WithExecutors` / `AddExecutor`：

- [tenant/skills HTTP](../../backend/internal/transport/http/openapi/skills/invoke_handler.go)
- [tenant/invocations 的 skill adapter](../../backend/internal/transport/http/openapi/capability_registry/tenant_handler.go)
- [SkillInvokeService gRPC](../../backend/internal/transport/grpc/skills/invoke_service.go)
- [Workflow Skill node](../../backend/internal/app/shared/workflow_runtime_adapters.go)

该构造器的 executors 初始为 nil，执行时最终返回 `no executor matched for skill`；没有把 payload 冒充成功结果。Agent 主路径已迁移为 DefinitionInvokeService，但这些入口与 Registry 发布、插件 Registry Sync 仍使用旧索引体系。

整改要求：在 T199 明确旧版本/旧调用方式的迁移或拒绝语义，后续统一接入发布 Definition Revision 与同一授权/依赖链。不能静默回填旧 Registry 成为运行定义，也不能用测试注入 executor 证明生产双路径一致。

### P23-B5：Skills 集成测试包仍引用已删除的旧插件 executor

`backend/tests/integration/skills/skill_plugin_bridge_failfast_integration_test.go:37` 引用不存在的 `NewPluginSkillHTTPExecutor` 和 `StaticPluginEndpointResolver`，当前整个集成测试包不能编译。只指定某个 `-run` 也会先编译整包，不能绕过这个失败。

后续应把旧夹具迁到当前声明式 Revision / Capability 调用链，不恢复已删除的业务专用 executor 来让旧测试通过。该失败阻止把旧的双路径和插件桥接勾选项作为本轮完成证据。

## 4. 本轮验证与证据等级

从 `backend` 执行的验证：

| 验证 | 结果 | 能证明的范围 |
|---|---|---|
| `go test ./internal/service/skills ./pkg/corex/agent/evidence ./internal/server/agent/bootstrap ./internal/service/agent -count=1` | 通过 | 发布定义、依赖失败关闭、内置计算和生产装配相关本地测试。 |
| runtime 的 Manifest/Evidence/Definition/ResponseEnvelope/Skill 定向测试 | 通过 | 相关 Runtime 本地回归；不是完整 Runtime 全套或真实模型验收。 |
| runtime 三项 MarketingReview 定向测试 | 通过 | 现有业务 ID 路由及绑定检查确实存在；不是 FR-079 合规证明。 |
| 对 skills/evidence/agent bootstrap/runtime/agent service 执行 `go vet` | 通过 | 相关包静态检查。 |
| Skills 调用 / 插件桥接集成测试包 | 构建失败 | P23-B5 中两个未定义符号，未进入集成用例执行。 |

集成包失败的复现命令：

```bash
go test ./tests/integration/skills -run 'TestSkillInvoke|TestSkillPlugin' -count=1
```

没有在本轮执行真实 LLM、插件算法、正式 HTTP/SSE、浏览器刷新、生产 grant 或数据库 seed；不复用历史已勾选项目作为本轮验收证据。相关通过结果不等于全仓测试通过。

## 5. 后续交付顺序

1. T199 先冻结外部工具依赖、版本/Schema/权限、可信执行回执、对象引用和旧 transport 的迁移合同，同时明确 P23-B1 的声明式任务选择边界。
2. T200 完成真正的生产 checker 与发布/绑定/执行接线，并明确部署可用 executor；不能只在测试中传回调。
3. T202/T203 完成外部结果来源与 ledger、失败节点、统一响应及历史/Trace 接线；不能扩展 computed 字段后让模型自行填工具结果。
4. T204 将示例及客户自建非营销 Skill 接入同一机制，解除业务 ID 路由分支；修复 P23-B4/P23-B5 的入口与夹具。
5. T205 用真实插件与变化数据、拒绝/超时、历史和浏览器回读完成业务验收，分别报告源码、本地、安装授权和端到端证据。

Phase 24 的 031 包含已落地源码和仍未完成的验收；本轮按用户选择仅推进 Phase 23，不据旧文字“未实施”回退或重做 031。
