# T204：示例 Revision 与客户插件工具

日期：2026-10-15。后续变化数据、真实模型与前端原件回读结果见 [T205 验收记录](evidence-acceptance.md)。承接 [T203](runtime-v5-implementation.md)。本次使用用户选择的本机 `base.local` 源码新增非营销示例；验证启动的是隔离安装目录中的真实原生进程，现有 Core/base.local 服务和数据库未被操作。变化数据、浏览器及当前安装环境验收继续由 T205 承接。

## 固有示例升级

`seed_native_marketing_agents.go` 的汇总示例现在生成 V5、tools/v2 和当前构建的 typed calculator 描述，策略升级为 calculation-policy/v3；全部输入明确 scope，公式明确结果单位。其余三个子任务仍输出 Markdown，不承担最终计算。

原有已发布定义不原地改写。seed 比较声明的 JSON 语义，变更时追加 Revision、生成不可变发布包，再发布新 Revision；相同声明重复运行不新增 Revision。工具源包或 Go 工具链改变也需要新 Revision，不自动接受实现摘要漂移。

seed 在变更前预检全部示例，数据库中的追加和发布在同一外层事务内提交。归档表缺失、预检失败或对象存储写入失败时，保留原已发布 current/status。事务失败可能留下未引用的不可变存储对象，不会把它当作已发布版本。本次仅在临时 SQLite/存储夹具验证，没有执行实际 `make seed`。

## 客户自建 Skill 的通用路径

```mermaid
flowchart LR
  A[客户 Skill 的固定 Revision] --> B[发布 / 绑定 / 执行预检]
  B --> C[正式工具合同与当前权限]
  C --> D[安装包中的原生进程固定副本]
  D --> E[插件领域算法和 typed 输出]
  E --> F[Core 回执 / V5 / 同次 ledger / 受控归档]
```

示例能力为 `com.powerx.plugins.base.parcel.volume@1.0.0`，输入长、宽、高，插件用精确十进制计算体积。领域算法位于 Framework skeleton 的 `internal/services/parceltool`；完整插件和无数据库的 `cmd/parcel-tool` 共用该 service/handler。Core 没有按该能力、Skill 名称或包裹业务分派的代码。

新 Revision 的 executor 为：

```json
{
  "type": "capability",
  "capability_id": "com.powerx.plugins.base.parcel.volume",
  "output_mode": "response_envelope",
  "response_contract": "powerx.agent.response/v5"
}
```

其 `tool_dependencies` 必须按 [T199](contracts/tool-evidence-contract.md) 固定 capability UUID、contract UUID、工具版本、provider 版本、两份 Schema 引用和摘要、实际实现摘要，以及 `admin_user` 权限 `parcel.volumes:compute`。不能把此示例的 UUID/摘要当作跨安装环境常量。完整创建 Source → Draft → 发布不可变包 → Publish → Agent 可运行检查的可执行例子在 `backend/tests/integration/skills/pinned_base_tool_integration_test.go`。

本期仅支持单个无副作用的纯计算工具。对业务数据的读取、写入、多工具 LLM 计划、远程 dev 挂载、脚本、Workflow、service_actor 均未开放，不会转到任意 URL 或旧 executor。

## 合同、路径和调用身份

新增 [plugin-tool-contract/v1](contracts/plugin-tool-contract.v1.schema.json)，实例在 Framework `skeleton/contracts/tools/parcel-volume.v1.json`。它包含输入/输出 Schema、字段单位/scope、双语标签和纯计算声明；插件不返回 V5、授权决定或成功回执。

- 初期输入为闭合顶层对象，1～32 个必需 number 字段；实际输入只接受最多 80 字节的普通十进制词面值。输出为同样闭合的十进制字符串对象，避免先转浮点再保存。
- 必须显式 `additionalProperties: false`。不支持的 Schema 关键词、嵌套、远程 `$ref`、重复字段、额外字段和伪造 receipt 均拒绝，不忽略关键词。
- 输入/输出的 Schema ID 分别指向 wrapper 中对应定义；两份 SHA256 均固定整个合同文件原始字节，因此字段语义和标签同样不可漂移。只读取已安装目录内 `contracts/` 下的普通文件，不网络拉取 Schema。
- descriptor 的 `metadata.protocols.http.schema_ref` 显式指向该文件，SyncWorker 保留它。同步目录和租户 registration 不等于创建正式 CapabilityContract/IOSchema、发布它们或授予 Agent 权限；这些治理步骤仍须先完成。
- HTTP 合同只写插件相对路径 `/internal/tools/parcel-volumes`。实际网络路径统一由 `PluginHTTPRoute(plugin.HTTPBase, relative)` 得到；Core API prefix 与插件 base 分属不同配置。目录中的 `/_p/<pluginID>` 是既有逻辑坐标，不能用作实际网络地址。
- 插件清单显式 `runtime.pin_executable: true`。Supervisor 校验独立 Go 可执行文件，将实际字节复制到私有只读文件再启动；拒绝 `go run`/Go 解释命令和脚本，禁止自动重启替换实现。Manager 校验已安装根目录、启动入口、版本、健康进程 generation 和摘要。
- 真实调用期间固定进程，阻止 Stop/Restart 穿插；前后检查 generation、目录快照、权限与 Schema。崩溃、重启、漂移或撤权都不能把结果封存为成功。
- 发布检查发布者权限，绑定及每次调用检查租户 registration、目标 Agent grant、当前用户 RBAC。上下文自报 root/用户/授权无效。通过后由 Core 注入私有 internal token 和可信租户/用户头；插件验证该通道凭据及 typed 输入。JWT 或 public-route 标记不能替代内部凭据。

此工具由 Core 的用户态 Agent 调用插件，不是 Framework 调用 Core 底座能力，不新增 Core 平台 service grant/API-key 权限。现有平台能力合同未修改。

## 原始结构化输入

Planner 产生的参数不能自己成为“用户输入来源”。本示例需要用户请求明确携带以下版本化声明，数字用字符串，避免 HTTP/WS/持久化先转换为 float64：

```json
{
  "schema": "powerx.agent.structured-tool-input/v1",
  "values": {"length_cm": "12.5", "width_cm": "4", "height_cm": "2"}
}
```

同步 Agent invoke 放在 `meta.tool_input`，typed StreamChatRequest 放在 `context.tool_input`，SSE GET 以 JSON 字符串放在 `tool_input` query（应由客户端做 URL 编码）。契约见 [structured-tool-input/v1](contracts/structured-tool-input.v1.schema.json)。现有界面未增加录入表单，实际浏览器接入不在本次验收内。

入口冻结值和当前主体为私有 context 权威；Planner 的普通 payload/context、同名字符串 key 或历史 JSON 不能创建此权威。执行时只从冻结来源选取合同字段，Planner 提供不同数字即在 HTTP 前失败。输入仍标记 `user_supplied_unverified`，计算不升级其真实性。

持久化只保存声明和原始字符串，不保存私有信任标志、JWT 或 token；Worker 完成当前身份、租户、Agent 和 Skill 校验后才重新建立来源快照。旧 Worker 无法理解新增受控 context key 时明确拒绝。受信任的直接结构化调用可使用 `ExecuteStructuredInput`；Agent 生产装配仍使用普通 `Execute`，不在 Planner 后自动授信。审批恢复若没有原始声明，继续拒绝，不从计划补造来源。

## 回执、历史和旧入口

Core 保存真实请求/响应字节、摘要、实际 provider/实现、目录快照和当前授权主体，输出 `domain_algorithm` derivation；同一 ledger 校验后才能封存 V5。归档增加可选 `agent_uuid` 元数据，不修改冻结 V5 报告/回执 Schema，也不新增数据库列。

插件历史回读仍限定原用户/租户，且实时检查原 Agent grant、租户 registration 和当前用户 RBAC。撤权后原用户/root 也不能查看；恢复权限后，可以读取冻结原件。历史不要求旧进程仍运行，不重新调用工具、不重算、不补抓当前业务数据。

旧 Registry 传输没有 Definition Revision/证据合同，统一明确返回 `skill.definition_contract_upgrade_required`：HTTP 409，gRPC FailedPrecondition，并记录 failed audit。不复活已删除的插件桥接 executor，不把 Registry SemVer 偷换成 Revision UUID。调用方须先发布/绑定新 Revision，再通过 Agent Definition 执行链调用。

## 验证记录与复现

以下命令从 Core 仓库根执行。真实插件源码不存在时集成用例会 skip；本次使用本机 Framework 路径实际执行通过，未 skip：

```sh
POWERX_T204_BASE_ROOT=/path/to/PowerXPlugin go -C backend test -v ./tests/integration/skills -run 'TestPinnedBasePluginCustomerSkillActualProcess|TestPluginSkillBridgeLegacyTransportRequiresDefinitionUpgrade' -count=1

go -C backend test ./internal/service/capability_registry -run 'TestBaseToolDescriptor|TestToolProtocol'
go -C backend test ./tests/integration/skills -run 'TestSkillInvokeDirect.*RequiresDefinitionUpgrade|TestSkillInvokeUnifiedPath|TestLegacyGRPCRequiresDefinitionUpgrade' -count=1

go -C backend test -race ./internal/infra/plugin/manager/supervisor ./internal/service/skills ./internal/server/agent/runtime ./pkg/corex/agent/evidence
make capability-check
python3 deploy/powerx/container/upgrade_registry.py --check
```

已验证：

| 项目 | 证据 |
| --- | --- |
| 正式插件描述导入 | 实际 split YAML 解析、descriptor/权限声明、SyncWorker 导入和 schema_ref 回归 |
| 真实 native 工具 | 实际构建/启动 base 工具，经 Manager/Supervisor 调用；不是 httptest 上游 |
| 通用业务接入 | 两个不同名称的客户 Skill 各走完整 Source/Draft/Publish/Invoke 链 |
| 动态 HTTP base | `/api/v1`、`/api/v2`、`/gateway/custom` 三组共 6 次真实成功调用，12.5×4×2=100 |
| 来源与权限 | 缺入口快照、伪造 Planner 数字和撤权均拒绝，实际插件调用次数不增加；原始长十进制持久化不丢精度 |
| 历史 | 新 Service 回读、撤权拒绝、恢复后旧进程已停止仍可回读 |
| 实现固定/清理 | 脚本和 Go 命令拒绝；原文件替换不改变私有镜像；Stop 等待调用完成，进程与端口释放 |
| 示例发布 | 重复 seed 幂等、存储失败回滚 current/status、缺表前置拒绝 |
| 插件领域与通道 | 精确小数/长数字、非正数、类型/重复/额外字段、内部凭据和主体拒绝 |
| 结构与构建 | 6 个真实 V5 DTO 符合冻结 Schema；相关 Go 单测/race/vet、Core 与完整插件构建、manifestcheck、capability-check、升级清单检查 |

Skills 集成包全量仍未通过：旧匹配/性能夹具期待无 executor 也成功，旧发现夹具缺 `executor.prepare_capability`；另有 Agent fixture 缺 session/message Trace、聊天表 UUID 列及运行环境相关断言失败。上述不算 T204 全量通过，也不恢复旧透传来满足夹具。T205 需要迁移这些过时夹具并完成实际安装环境、不同数值/失败情形和浏览器刷新验收。

临时测试进程全部由测试记录 PID、命令、cwd 和动态端口，在 cleanup 里停止并确认端口释放。当前用户服务未重启，实际数据库未迁移，现有示例 Revision/grant 未更新，公网与浏览器未验收。
