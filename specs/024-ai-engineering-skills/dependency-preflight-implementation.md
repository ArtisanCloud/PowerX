# T200：发布、绑定、执行前依赖预检

日期：2026-10-14。按 [T199 合同](contracts/tool-evidence-contract.md) 落地治理态 Definition 的三阶段预检；本轮不启用 V2 工具执行、typed 计算或 V5，不将测试替身视为真实插件验收。

后续状态：以下是 T200 交付时的检查点。[T202](source-tool-evidence-implementation.md) 已实现 typed 内置执行/可信回执基础层，并给生产预检装配真实内置描述校验；[T203](runtime-v5-implementation.md) 已接线显式新 Revision 的 V5 内置链与受控历史（部署须迁移），外部 pinning 仍未开放。下文“没有 PinnedTools/typed adapter”仅指 T200 当时，不代表 T202 后的内置实现状态。

## 1. 生产接线

| 阶段 | 调用点 | 检查与写入边界 |
| --- | --- | --- |
| Definition 发布 | `DefinitionService.PublishCurrentRevision`；原生 Skill seed 使用同一生产 factory | 读取当前 Revision 后预检，失败不发布。发布事务锁定 Draft 并比对已检查 Revision UUID，发生并发追加时返回 `skill.definition_revision_conflict`，不发布未经检查的新版本。 |
| Agent 可运行绑定 | `AgentService.ReplaceSkillBindings` → `CheckRunnableForAgent` | 传入实际 env/Agent ID，全部 Skill 检查成功后才写 binding。失败保留原有绑定，不创建 active 的残缺绑定。 |
| 每次执行 | Agent/child handoff 的 `DefinitionInvokeService` → `ManifestExecutor` | 每次重新读取发布定义、目录、最新 registration 和实时授权，预检失败不进入 LLM/Capability/Workflow 调用端口。 |

共享服务为 [DependencyService](../../backend/internal/service/skills/dependency_preflight.go)，生产装配集中在 [NewRuntimeDependencyService](../../backend/internal/service/skills/dependency_runtime.go)。`shared.Deps.SkillDependencies` 注入管理端 Agent 服务；bootstrap、seed 及非 HTTP 调用方使用相同 factory。检查服务不保存或复用“曾经允许”的决定，也不使用业务 Skill/Agent/Team ID 分派。

目录 IO 由 [ToolCatalogRepository](../../backend/pkg/corex/db/persistence/repository/skills/tool_catalog_repository.go) 完成，服务依赖其窄接口；查询同租户合同、按版本降序取得**最新** registration，不使用旧 active 记录或工具缓存替代。读取事务不是执行版本锁，实际 pinning 仍归后续 adapter。

## 2. 检查规则

- tools/v2 严格解码，拒绝额外字段、缺字段、空工具集合、重复局部 key/工具 key、非精确 SemVer、非法/零 UUID、不完整 Schema/实现摘要和非法 actor/protocol。依赖对象最多 1 MiB、64 项；通用 JSON 解码拒绝重复字段、非法 UTF-8、尾随 JSON 和过深嵌套。
- 工具机器 key、能力 UUID、同租户合同 UUID、工具合同版本、provider 版本、Schema ID/摘要逐项核对；未发布、尚未生效或已失效的合同阻断。
- registration 必须是本租户最新的已发布可用记录。`contract_ref` 保持现有**工具合同版本字符串**语义，与合同 version 精确比较；不将该既有字段改写为 UUID。UUID 通过 `(tenant_uuid, capability_key, version)` 唯一合同另行校验。
- 正式权限 code、Agent 可用性、目录 protocol/actor、合同 transport profile 和 registration 中 active adapter 必须一致。`core_internal` 必须仍是固定 `core://` + `INVOKE`，不放开任意 HTTP 代理。
- 发布按当前发布者的可信用户上下文与 RBAC 检查；绑定/执行额外检查目标 Agent 的实时 grant。身份从 reqctx 读取，不接受 Skill 的 payload/context map 自报 root/user/member。
- 当前生产只接入用户态授权检查。service_actor 工具明确拒绝，不能借用发布者用户 RBAC；服务态 grant/API-key/delegated 的真实执行适配必须按正式合同另行接入，不能用测试回调或宽授权替代。
- 不支持 executor 的定义可保存为不可运行 Draft；Workflow、脚本与 instruction_only 不得通过当前可运行发布/绑定。已有普通 LLM 与 tools/v1 + V4 内置计算保留原义，旧 key-only 外部工具和缺依赖的 capability 声明明确阻断。

新错误通过 `DependencyError` 保留 code/stage/dependency key，底层 SQL 错误作为受保护 cause，不直接拼入用户可见错误。Trace/locale 的最终失败节点与消息投影仍由 T203 统一接入。

## 3. 尚未开放的端口

tools/v2 能被解析和校验，不等于当前部署能执行它。生产 factory **没有装配 PinnedTools**：

| 情况 | 当前行为 |
| --- | --- |
| 合法外部工具，目录和用户/Agent 权限都满足 | `skill.tool_protocol_unavailable`，零次工具调用；缺少实际版本/实现/Schema 固定的执行端口。 |
| evaluate@2.0.0 的合法 typed 声明 | `skill.tool_dependency_unavailable`；typed adapter 尚待 T202，不复用旧版本名冒充新合同。 |
| V5 或 LLM 外部工具编排 | 当前部署不标为可运行；不能用 V4 计算链或普通 Prompt 隐式消费新依赖。 |
| 旧 tools/v1 外部声明 | `skill.tool_contract_upgrade_required`，要求形成新 Revision；无旧 executor fallback。 |

后续只能在实际 invoker 确实消费同一固定快照时装配 PinnedTools。目录的 capabilities_hash、provider 自报实现摘要、一次预检或 HTTP 200 都不是实现验真。T202/T203 还需接入真实实现/版本锁定、对象来源、输出 Schema/Pointer、可信回执、ledger 和 V5 消费链；T204/T205 再验证客户自建 Skill 与真实插件。

本轮也未迁移 T198 中四个旧 Registry transport/Workflow adapter，未修复旧 Skills 集成夹具的已删除符号，未移除营销任务选择特例。旧 Registry 的 published 是目录状态，不能据此称其通过了新 Definition 的 runnable 检查；这些缺口继续按 T203/T204 推进。

## 4. 回归与证据范围

新增测试涵盖：

1. 三阶段对缺工具、版本/provider/Schema 漂移、跨租户合同、未发布/未生效、不可用于 Agent、未声明权限、缺协议、禁用 transport/adapter、最新 registration 禁用的拒绝。
2. 发布可通过的测试部署，在 Agent grant 撤销后，绑定和执行重新拒绝。通过 `Draft → Publish → CheckRunnableForAgent → DefinitionInvokeService → 模拟工具` 串联，首次调用成功、撤权后不再调用模拟工具。
3. 无 pinning 端口的真实生产 factory，即使 root、目录与 grant 合法也拒绝；service_actor 无法借用用户权限，自报身份不能生效。
4. AgentService 实际写入入口先执行注入检查，失败不落新绑定且不覆盖旧绑定；未知 executor/Workflow/instruction_only 保留 Draft，不被发布为可运行。
5. 发布预检后发生 Revision 变更时拒绝提交；旧内置 V4 回归保持通过；JSON 重复字段、未知字段、尾随内容、非法编码和深度限制拒绝。

本地测试使用内存 SQLite 的真实仓储/授权服务；Agent 表使用适配 SQLite 的夹具，相关 PostgreSQL cast/外键 DDL 不用于 SQLite。正向外部工具用例显式替换 pinning 端口与工具本身，**没有执行真实插件或证明生产版本锁定**。并发发布用例验证 Revision 比对分支，不替代 PostgreSQL 多连接锁竞争验收。

从 `backend` 执行：

```sh
go test ./internal/service/skills ./internal/service/agent ./internal/service/agent_authz ./internal/server/agent/bootstrap ./pkg/utils ./pkg/corex/agent/evidence ./cmd/database/seed -count=1
go vet ./internal/service/skills ./internal/service/agent ./internal/service/agent_authz ./internal/server/agent/bootstrap ./internal/app/shared ./internal/transport/http/admin/agent ./pkg/utils ./pkg/corex/db/persistence/repository/skills ./cmd/database/seed
```

相关测试与静态检查通过，Core `./cmd/app` 构建通过，临时构建目录已清理。未运行全仓测试。复核 `go test ./tests/integration/skills -run 'TestSkillInvoke|TestSkillPlugin' -count=1` 仍在构建阶段失败：`skill_plugin_bridge_failfast_integration_test.go:37` 引用已删除的 `NewPluginSkillHTTPExecutor` 与 `StaticPluginEndpointResolver`；没有执行其中的集成用例，也没有为通过旧测试恢复专用 executor。

本轮没有修改数据库结构、执行 seed/migrate、写入实际 grant 或重启用户服务。加载修改后的后端需由用户按日常流程重启；真实安装授权、HTTP/SSE、失败节点与历史回读验收留待后续任务，不能复用本地测试作为这些环境的证明。
