# T203：V5 Runtime、历史与失败状态

日期：2026-10-14。后续原生插件计算接线及示例 Revision 代码见 [T204](plugin-tool-example-implementation.md)；本页未开放项为当日快照。承接 [T199 合同](contracts/tool-evidence-contract.md)、[T200 预检](dependency-preflight-implementation.md) 和 [T202 来源核验](source-tool-evidence-implementation.md)。本页记录源码接线及本地验证；实际安装插件、自建插件工具与浏览器业务验收属于 T204/T205。

## 运行合同

只有显式发布的新 Revision 可以选择下列组合，既有 V4 不会自动升级：

| 声明 | 新链要求 |
| --- | --- |
| Definition | `powerx.skill-definition/v2`，固定 Revision UUID |
| executor | `llm_prompt` + `output_mode=response_envelope` + `response_contract=powerx.agent.response/v5` |
| 工具依赖 | `powerx.skill-tools/v2`，实际内置 `powerx.calculation.evaluate@2.0.0` 的实现及 Schema 摘要 |
| 计算策略 | `powerx.skill-calculation-policy/v3`，每个输入显式 `scope_i18n`，每个公式显式 `result_unit`；无量纲为 `1`，percent 对应 `%` |
| locale | 当前仅 `en-US`、`zh-CN`；发布声明的每种 locale 必须补齐画像、字段、口径、公式和 review 文案 |
| 存储 | 受控证据表已经迁移且可用；发布、绑定、执行均检查，缺失时在调用 LLM/工具前拒绝 |

策略 v2 保留旧含义，不能根据表达式或营销字段猜测新结果单位。正式外部 adapter、业务对象 resolver、service_actor 历史授权和多工具 LLM 计划仍未在生产装配，继续拒绝；不能把内置计算接线当成外部插件验收。

实际路径是：固定 Revision → 统一预检 → 冻结输入 → 选择现有 token → 编译声明字段和公式 → `ResolveDatum` → typed 实际调用 → 核对结果/derivation → `SealReportV5` → 同次 Runtime ledger → 受控归档 → 实时/历史展示。没有中间 V4 报告转换，也没有历史导入 ledger 的入口。服务端通过 typed invocation scope 传 run/node，主体和租户来自可信 reqctx；模型 context map 不提供这些身份。

Core 已移除营销 Skill ID 的确定性任务替换。任务使用现有通用 planner 和发布定义，不增加新的业务分派规则。固有示例 Revision 与旧 transport 的后续整改仍归 T204。

## 响应与展示

`ReportV5` 与 [冻结 Schema bundle](contracts/tool-evidence.v1.schema.json) 对齐；运行 Schema 从同一原件导出到 `backend/config/agent/response_envelope.v5.schema.json`。`reported/computed/conflicts/gaps` 保留不同语义，computed 只引用实际成功回执和真实输出；输入状态不会因为计算而升级为事实。

以下 metadata 与 V5 报告分开版本化，不向 V5 添加私有字段：

| metadata | 内容 |
| --- | --- |
| `response_labels` | `powerx.response-labels/v1`，发布 Revision 的字段/公式 locale 快照 |
| `response_evidence` | `powerx.run-evidence/v1`，受控归档 UUID 与 SHA256 引用 |
| `response_evidence_state` | 历史投影的 `available` / `unavailable` |
| `failure_stage` / `reason_code` | 系统失败的稳定阶段/代码，与任务和 Trace 一致 |

Web Admin 使用平台 locale 标题、状态、冲突原因及原 Revision 标签；不把工具 ID、回执 UUID 或机器字段 key 作为业务标签。展示只给已保存的十进制值补小数位，不重新计算、换算、取整或重新抽取历史原文。hypotheses/actions 保留执行时生成的语言。新旧响应按明确 Schema 分派，旧 V4 继续读取，未知版本不冒充 V5。

## 受控归档及权限

新增模型 `agent_evidence_snapshots`，集中挂载到 `migrateAgentServiceSessionModels`，内容寻址升级清单已重新生成。字段包括租户、用户主体、Revision、run/trace、归档摘要、`payload BYTEA`、status 及公共 UUID/软删除字段。

- `payload` 使用二进制保存完整归档原件。不能改为 JSONB：PostgreSQL 会重排 JSON，破坏精确字节摘要。
- 归档中的来源、请求和输出为 base64 编码字节，避免 JSON 编码器压缩空格或改写转义；摘要针对原字节。
- 写入须来自当前 Core `ToolRun` 私有导出权威，且导出后未被修改。历史 JSON 即使结构和摘要合法，也不能再次保存成新执行。
- 当前只允许用户态 `run_input` 与内置工具输出，严格按原租户和用户读取；root 身份不能借阅另一用户的证据。正式对象/外部工具需要补相应实时授权端口才可开放。
- 历史服务核对归档摘要、来源定位、报告/回执关联及消息报告一致性，并恢复归档中的 Revision 标签；不信任消息中被替换的标签。
- status=0、软删除、硬删除、无权、摘要损坏、消息篡改或旧快照缺失均移除报告及标签，显示“证据不可用”。不会重新查当前对象、重新调用工具或重建旧报告。当前没有新增自动保留期限/清理任务。
- 每 run 私有字节上限仍为 64 MiB，归档上限 96 MiB 用于 base64 开销。失败时尽力保存实际部分调用和稳定 failure，保存失败不覆盖最初错误，也不生成成功报告。

本次只交付迁移代码，**没有操作实际数据库**。由用户部署时在仓库根执行 `make db-migrate`，再自行重启相同构建的 Core/Worker；未迁移时新链失败关闭。没有运行 seed，也没有更新现有固有 Revision 或实际授权。

## 内置实现摘要修正

T202 曾按整个可执行文件计算摘要；跨 database/app/Worker 入口会产生不同标识，不能用于共同固定内置工具。T203 改为随构建嵌入 evidence 包全部生产 Go 源文件、实际严格 JSON 实现和 Go Runtime 版本，生成固定实现包摘要；测试文件不进入包，Schema 摘要仍来自完整正式原件。描述由可信构建生成，不使用可变目录标签或 provider 自报值。

因此同一源包和工具链的不同入口二进制得到相同描述；工具包或 Go 版本改变后，依赖它的新链 Revision 必须显式更新并重新发布，不自动接受漂移。回归实际启动另一入口的 `go run`，比较其描述与测试二进制，同时核对包中源字节。

## 业务与系统失败

| 情形 | 行为 |
| --- | --- |
| 缺操作数、单位/口径不一致 | typed gap、`needs_action`，不调用该公式，不伪造失败回执 |
| 原文与计算冲突 | 同时保存两个值及冲突，`needs_action`；`2.50` 与 `2.5` 精确数值相等，不制造冲突 |
| 除零、工具失败、撤权、证据/存储错误 | 任务 failed，稳定 code/stage；不能归为业务待补或成功报告 |
| 最终结构或 ledger 验证失败 | 实际 final validation 失败节点，实时 failed node/final/error/end；持久化任务也记录失败节点 |

持久化 Worker 将 typed failure 的 code/stage 写入 `TaskSnapshot` 和 `agent_run.task_failed`。历史最终消息、任务快照和 Trace 投影保留相同值；新增 `failure_stage` 是已有状态协议的可选字段，没有新增调度状态或重试器。旧非证据错误保持现有处理。

## 本地验证

回归覆盖实际 Draft → Publish → DefinitionInvoke → typed 内置计算 → SQLite 受控归档 → 新 Service 只读回显；同时验证跨用户/租户拒绝、历史重存拒绝、消息篡改、归档清理、缺参、冲突、等价小数、口径不一致、除零失败及迁移/策略/locale 及匿名主体预检。Unicode 空白后的百分号不能被截掉冒充数量，零宽格式字符不能拆分数值和单位。

另有实际 Local Trace 失败节点和实时/历史错误一致性测试，以及 miniredis Worker → task/event 的失败码/阶段测试。前端包含 V4/V5 locale 渲染和 durable 刷新连接回归。

```sh
# backend 目录
go test -race ./pkg/corex/agent/evidence ./internal/service/skills ./internal/service/agent_run ./internal/server/agent/runtime
go test ./internal/service/agent ./internal/service/agent_evidence ./internal/service/agent_session ./internal/server/agent/bootstrap ./internal/service/agent_authz ./pkg/utils ./cmd/database/seed ./pkg/corex/db/database
go test -v ./pkg/corex/agent/evidence ./internal/service/skills > /tmp/powerx-t203-contract.log

# 仓库根
uv run --no-project --with 'jsonschema[format]==4.23.0' python specs/024-ai-engineering-skills/contracts/validate_tool_evidence.py --go-test-log /tmp/powerx-t203-contract.log
python3 deploy/powerx/container/upgrade_registry.py --check

# web-admin 目录
npx vitest run tests/unit/agent/response-envelope-v5.spec.ts tests/unit/agent/response-envelope.spec.ts tests/unit/agent/durable-refresh-connection.spec.ts tests/unit/agent/history-message-meta.spec.ts
bash scripts/check-refactor.sh
npm run build
```

相关 Go 包、race、vet、Core app 构建、前端 21 项测试、目录检查及 Nuxt 生产构建通过。离线校验验证 62 项结构用例与 22 个实际 Go DTO；41 项 runtime_cases 在 Python 中仍只检查声明，不计为端到端执行证明。没有运行全仓测试、真实 PostgreSQL 迁移、真实插件或浏览器业务验收；旧 Skills integration 包的过时 executor 引用继续归 T204 清理。本轮没有启动或重启用户服务。
