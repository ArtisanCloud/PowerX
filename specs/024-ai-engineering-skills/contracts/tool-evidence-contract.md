# T199：版本化工具依赖与结果证据契约

状态：2026-10-14 冻结的 Phase 23 实施合同。本文与 [JSON Schema](tool-evidence.v1.schema.json)、[契约用例](tool-evidence-cases.json) 共同约束 T200、T202～T205。[T200 三阶段预检](../dependency-preflight-implementation.md) 已落地；V2 工具执行、可信回执及 V5 仍未开放，不能把合同解析能力当成可运行交付。实际起点见 [T198 审计](../tool-runtime-audit.md)。

## 1. 版本与实施边界

| 对象 | 当前生产格式 | 本次冻结的目标格式 | 启用条件 |
| --- | --- | --- | --- |
| Skill 定义 | `powerx.skill-definition/v2` | 保持 v2 外层；`tool_dependencies` 显式使用 `powerx.skill-tools/v2` | T200 严格识别嵌套版本并装配检查器。未知版本拒绝，不能按 v1 读取。 |
| 工具依赖 | `powerx.skill-tools/v1` | `powerx.skill-tools/v2` | 固定工具、契约、实现摘要、协议和权限；三个检查阶段全部接线。 |
| 工具执行回执 | V4 内置计算记录 | `powerx.tool-receipt/v1` | T202 由受信任执行端口产生、校验并写入本次执行 ledger。 |
| 内置计算端口 | evaluate@1.0.0 声明 draft/v1 → response/v4 | evaluate@2.0.0，`powerx.calculation.request/v1` → `powerx.calculation.result/v1` | T202 用 typed adapter 复用已有受限算术；新输入输出不能复用旧工具版本名。 |
| 最终证据报告 | `powerx.agent.response/v4` | `powerx.agent.response/v5` | T203 同步 DTO、Runtime、locale、历史及渲染器后启用。普通工具仍返回自己的输出，不实现聊天信封。 |
| 输入、公式、解释策略 | 现有严格阶段 Schema 与 calculation_policy/v2、review_policy/v1 | 本次不修改其字段含义 | 外部工具新增阶段时单独版本化，禁止直接让旧模型阶段填写回执或完整 V5。 |

V4 的 computed 只能描述内置计算器，不能承载通用外部回执。V5 新增结构化来源、UUID 数据关联、固定工具身份及回执引用，属于显式破坏性升级；不修改 V4 Schema 或把旧报告“补字段”变成 V5。

本目录的 Schema 是设计合同，不放入 `backend/config/agent` 冒充生产已支持。一个离线 JSON Schema bundle 通过各对象的 `schema` 字段判别五种 wire 格式，使用 Draft 2020-12；所有平台对象均关闭额外字段。T202/T203 应以正式 DTO 导出运行时 Schema，并测试其与本合同一致；如确需变更合同，先更新版本及用例。

| wire schema | bundle 内权威定义 |
| --- | --- |
| `powerx.skill-tools/v2` | `#/$defs/tool_requirements` |
| `powerx.tool-receipt/v1` | `#/$defs/receipt` |
| `powerx.agent.response/v5` | `#/$defs/report` |
| `powerx.calculation.request/v1` | `#/$defs/calculation_request` |
| `powerx.calculation.result/v1` | `#/$defs/calculation_result` |

逻辑 Schema ID 由受信任目录映射到上述定义；引用摘要固定包含间接定义的完整 bundle 原件字节，不能只哈希带有未固定 `$ref` 的局部片段。合成夹具只展示结构和数值关系，其 UUID/实现摘要/授权决定不是可复制使用的生产对象或凭证。

这些版本与 HTTP 挂载前缀无关，不通过修改 `server.api_prefix`、插件 HTTP base 或 URL 中的 v1 来完成迁移。本文不新增 HTTP 端点、gRPC RPC、Core capability 或数据库字段。

## 2. 工具身份与精确依赖

`tool_dependencies` 必须声明 `schema` 与非空 `tools`。每项包含：

| 字段 | 规则 |
| --- | --- |
| `dependency_key` | 本 Revision 内唯一的局部机器 key，用于引用；tool_key 同样不允许重复，不充当权限或全局业务对象身份。 |
| `tool_key`、`version` | 稳定机器 key 与精确 SemVer；禁止 latest、范围、通配或运行时重新选择版本。工具版本与插件发布版本独立。 |
| `input_schema`、`output_schema` | 各自包含版本化 `id` 和 `sha256:` 摘要。只能解析受信任目录/发布产物中的 Schema，不按模型提交的 URI 发起网络或文件访问。 |
| `permission` | `actor` 与最小权限 `code`。声明是要求，不是授权结果；必须与正式工具合同和本次主体匹配。 |
| `binding` | `builtin` 或 `capability`。Capability 必须固定能力记录 UUID、合同 UUID、provider 精确版本、实现摘要及 protocol。 |

现有 `powerx.calculation.evaluate@1.0.0` 保留旧 draft/report 合同；新端口固定为 `powerx.calculation.evaluate@2.0.0`，用独立 typed 输入输出承载计算，不再把聊天报告当工具结果。它复用同一受限算术实现，不因数学算法相同而忽略 wire 变化。builtin 只接受部署清单列出的内置工具、Schema 摘要与 `runtime_internal/pure_calculation`；外部工具不得声明 builtin。新端口尚未实现，不能提前将其登记为可运行。

Capability 的 `capability_uuid` 对应正式能力目录记录 UUID；`contract_uuid` 对应发布合同 UUID。`tool_key` 必须与这两个记录解析出的机器能力 key 一致。现有 capability executor 的 `executor.capability_id` 保留机器 key 语义，必须且只能对应一项 capability 依赖；通过该项的 UUID、版本和授权执行，不能另走未固定的 key-only 路径。未被显式支持的多工具编排应拒绝，不能假定列出依赖就已具备模型工具调用能力。

Capability 依赖的 version 必须等于该 contract_uuid 对应的已发布 CapabilityContract.version；provider_version 另指插件/提供方发布版本。不存在正式合同或目录无法确认二者关系时阻断，不从插件版本猜测工具合同版本。最低 Runtime 要求由 tools/v2、executor 类型及声明协议的支持集合共同确定；缺任一已声明特性即不可运行，不能仅凭 Core 版本号较新推断支持。

协议枚举是可声明的名称空间，不是部署支持清单。部署必须另行确认该协议、executor、精确实现及版本固定能力已装配；当前 Workflow unavailable、缺连接的 gRPC、未接线的 MCP 或脚本不能因枚举合法而发布为可运行。协议不支持、Schema 摘要不明、实现摘要不能确认或运行端不能锁定版本时失败关闭。

版本匹配比较完整规范版本字符串及摘要，不能用忽略 build metadata 的兼容性比较代替固定版本。原始 JSON 还必须拒绝重复字段、尾随 JSON 与非法 UTF-8；这些字节级检查在 Schema 之前完成，不能因解码后的对象可通过 Schema 就接受歧义输入。

`implementation_sha256` 必须来自受信任安装/发布链对实际执行实现的证明，不能仅用可修改的版本标签或能力元数据哈希冒充二进制/实现摘要。builtin 也由部署清单固定实际实现。Capability 目录当前是可变记录，UUID 本身不代表不可变版本；要同时核对合同、版本、Schema 与安装实现摘要。

授权必须复用正式 capability 的主体、租户 registration 与实时 grant。Agent 的用户代理调用按用户权限及绑定评估；服务调用按 service_actor 及服务授权评估，两者不能互相借用。Core 能力供 Framework delegated Host 消费时，仍须遵守根目录 AGENTS 的 Admin/service 双授权面、typed core_internal 和显式 API-key 元数据要求。本次不新增或修改这些能力合同。

## 3. 发布、绑定、执行的检查时点

| 阶段 | 必须完成 | 失败行为 |
| --- | --- | --- |
| Draft 保存 | 保存原件、结构诊断及缺失项；未满足依赖仍可保留不可运行 Draft。 | 不写 published/runnable，不把未知 executor 自动改为 llm_prompt。 |
| 发布 | 严格 Schema、唯一 key、精确版本、UUID 对应关系、发布状态、Schema/实现摘要、部署 executor/protocol、租户 registration 和发布主体权限。冻结检查结果到发布产物。 | 不发布可运行 Revision；不修改已有发布版本。未来 Agent/用户的权限留待绑定与执行检查，不能用发布者授权代替。 |
| 可运行绑定 | 重新核对依赖可用性、Agent/Team 绑定边界及适用执行主体的授权。 | 拒绝 active/runnable 绑定，可保留明确 disabled 的配置。 |
| 每次执行和重试 | 读取同一发布 Revision，重新授权主体和对象；解析精确工具快照，执行前确认摘要与可用性，取得受信任输入快照。 | 不发起未通过检查的工具调用；不得用发布时缓存权限代替实时 grant。 |
| adapter 调用及返回 | 将同一固定快照交给 adapter；调用端必须实际固定实现/版本，返回后核对实际身份、Schema 和结果。 | 发生漂移/撤权/契约错误时记录失败。单纯“调用前查过版本”不解决检查到调用间的漂移。 |

检查器必须由同一依赖服务经 DI 注入发布、绑定和 Manifest executor，不能只在测试中传回调。事务边界、撤权与缓存失效实现归 T200；不能靠扩大全局锁或修改业务 Skill 名称绕过。

## 4. UUID 来源、快照与可信状态

V5 的 `reported` 是从来源定位解析出的数据集合，其中 `datum_kind=quantity` 是可作原始操作数的数量，`datum_kind=reported` 是原文已报告的比率/主张。两者都不自动获得“真实”或“归因成立”的结论。

每个 datum 含平台生成的 `datum_uuid`、Skill 字段 key、原始 `literal`、解析后的十进制 `value`、`unit`、`scope`、来源和输入状态。平台负责从快照解析数值；模型只能选择声明字段及合格来源，不得另填值或更改单位。

| 来源 kind | `object_uuid` 的含义 | 额外约束 |
| --- | --- | --- |
| `run_input` | 当前调用冻结输入对象的 UUID | 只能解析本轮显式提供的输入；HTTP/Workflow 没有 Chat message 时也必须先建立输入快照，不能伪造 message UUID。 |
| `authorized_object` | 本租户业务对象 UUID | `object_type` 必须命中注册的 typed resolver；校验读取权限、对象版本及字段范围。未知类型、跨租户或无权对象统一拒绝，不泄露对象是否存在。 |
| `tool_output` | 本次受信任执行产物 UUID | 必须含 `execution_uuid`，对应本 run、已完成且验真的回执和实际输出摘要。上游自然语言总结不属于该来源。 |

每项来源都固定 `snapshot_uuid/snapshot_sha256`、JSON Pointer、精确 quote 及 UTF-8 字节范围 `[start_byte,end_byte)`。范围相对于该 Pointer 解析出的字符串；结构化标量按受控快照的确定性 JSON 字节定位。对象无原生不可变版本时，resolver 必须先冻结读取结果。Pointer、quote、literal、范围、值与摘要必须相互核对，不允许用任意 URL、本地路径、SQL 或自由文本标签充当对象引用。

`input_state` 分为 `user_supplied_unverified`、`extracted_unverified`、`authorized_record`、`tool_result`，由受信任 resolver 设置。`authorized_record` 仅表示有权读取该版本记录；`tool_result` 仅表示来自实际执行。两者都不证明业务事实或归因已经独立核实。模型、调用方与上游 Skill 不能自行授予状态；引用复制不得提高等级。通用算术不能把未经核实的输入变成已核实事实。

UUID 关联包括来源对象、快照、datum、执行、Skill Revision、能力合同和授权决定。工具 key、权限 code、字段 key、协议名、Trace/run/node 标识以及基础设施 adapter_id 是机器语义或现有 opaque 标识，不强行转换为业务 UUID。

数字统一使用有限十进制字符串，不使用 JSON 浮点数承载金额。原始逗号、百分号与单位保留在 literal/quote；单位与 scope 不隐式转换。只有百分比时仅产生 reported，不能创造分子/分母。单位或人群/时间口径无法确认时生成缺口或冲突，不执行不具备前提的跨口径公式。

typed 计算请求包含受限 expression、变量到 `datum_uuid/value/unit/scope` 的实际解析结果、precision、percent、`half_away_from_zero` 和 result_unit；输出仅含 value、unit 与实际采用的精度/比例/舍入。输入数值由平台 resolver 填入并对照来源，不能直接信任模型提交的重复 value。表达式、单位及输出口径来自发布策略；typed adapter 不自行发明单位换算。继承既有算术的表达式 512 字节、32 变量、128 步、精度 0～12、输入词面值 80 字节及中间有理数 4096 bit 限制，输出十进制最多 1280 字符。每个完整协议对象的 UTF-8 JSON 最多 1 MiB；原始输入材料及外部产物另按正式工具/平台的更严格限制执行，不能只依赖数组 Schema 限制。

typed 输出 value 不带单位/百分号，规范化去除小数尾零并将负零写为 `0`，展示精度由 precision 保留。percent=true 表示先乘一百再舍入，result_unit/输出 unit 必须为 `%`；percent=false 不得声明 `%`。例如同源数量相除可产生百分比，但该计算不证明来源中的数量真实，只有原文百分比仍不能构造数量。Schema 只能校验字段关系，数值及舍入必须对照真实执行。

## 5. 真实执行回执

回执由 Core 的受信任执行边界产生，插件只返回其正式输出，模型不能填写回执。执行流程为：

```text
发布 Revision + 实时授权
→ 固定工具/adapter/实现与 Schema
→ 解析并授权来源、冻结输入
→ 调用工具，记录实际输入输出及执行时间
→ 校验实际版本与输出 Schema
→ 写入本 run 的受保护 ledger
→ 平台将已验真回执和结果引用投影为 V5
```

回执至少包括执行 UUID、租户、Revision、run/trace/node/attempt、dependency_key、固定依赖快照、adapter/registration 快照、授权决定 UUID、输入 datum、typed 请求产物及摘要、起止时间和状态。`succeeded` 必须有输出产物及摘要；failed/timed_out/cancelled 必须有稳定错误、失败阶段和可重试标志，禁止携带成功 result。

执行前检查失败仅记录失败节点和拒绝审计，不虚构工具调用或执行回执。builtin 的 subject_uuid 是平台运行主体 UUID，仍须有本次授权决定；它不代表登录用户，也不能获得外部数据访问权限。

请求/输出产物是受控存储中的 UUID 引用，摘要针对**实际校验和执行的冻结字节**，不能只哈希摘要标签。Schema 摘要针对受信任发布 Schema 原件；内容序列化及摘要约定必须由产物实现统一处理，不能由模型各自重序列化。回执中不存 JWT、API key、client secret 或任意可调用 endpoint。

SHA-256、UUID、`status=succeeded` 与 JSON Schema 合法都不提供真实性证明。Runtime 必须核对 ledger 中平台创建的记录、上下文、固定身份、输入/输出摘要、来源权限、输出 Pointer 和实际值；重放其它 run 的回执、伪造 UUID、篡改数值或插件声称已验真均拒绝。运行中读取依赖权限不能从 ledger 恢复。

最终报告嵌入所引用回执，以便历史离线展示及完整性检查；执行时的可信判定仍依赖受保护 ledger。历史回读只展示当时记录，不重授权执行、不重新计算、不把历史记录重新放入当前 ledger。保留/清理快照时必须同步标记证据不可回读，禁止重新抓取当前对象补成“原快照”。存储落点、完整性校验和访问控制由 T202/T203 实现；本任务不创建表。

本版本只消费同一 run 的执行记录；A2A 子 run 的文本或回执不能直接冒充本 run 的 ledger 记录。以后若开放跨 run 结构化证据，需要受信任 handoff 导入合同，保留原执行身份并重新核对来源授权；不能通过改写 run_id 适配。

## 6. V5 报告分类与计算语义

| 字段 | 唯一来源及校验 |
| --- | --- |
| `presentation.reported` | 解析来源快照得到 datum；保留原始材料及未独立核实状态。不能由生成的 narrative 再提取为来源。 |
| `presentation.computed` | 引用 succeeded 回执 UUID 和实际输出 Pointer；value 必须等于对应输出标量。表达式与 operand UUID 绑定或领域算法及参数摘要必须来自发布策略和实际请求。 |
| `presentation.conflicts` | 关联真实 reported datum UUID 与 computed datum UUID；保留值及不同原因，不能覆盖任一证据。不可比口径不能误报为纯数值错误。 |
| `gaps/hypotheses/actions` | 缺参、未核实假设与后续行动；受 Skill locale/策略约束，不能成为新的数值计算来源。 |

算术 derivation 明确 expression、变量到 quantity datum UUID 的绑定、precision、percent 和 `half_away_from_zero` 舍入。表达式禁止业务数值常量；百分比尺度由 percent 显式提供。平台内置纯计算可确定性复算；领域算法通过真实受授权输出、固定实现和参数摘要验真，不能用通用计算器假装独立复现任意领域算法。允许的公式、单位/口径关系均来自发布 Skill/工具合同。

V5 的成功业务报告仅允许 `outcome=completed|needs_action`。有 conflict、gap 或 hypothesis 时必须 needs_action；至少存在 reported、computed 或 gap。completed 只表示声明的执行步骤完成，不表示输入真实性或归因已核实。系统错误不伪造成 business outcome=needs_action：写标准错误及失败节点，可保留已验真的部分证据，但整轮不能显示绿色成功。

表格和技术名称由平台 locale 渲染；字段 label_key 在对应 Revision 的 locale 中解析。模型生成的说明使用本次请求 locale。普通工具结果和插件输出不需要平台章节标题，展示层不重算数值。

## 7. 错误、状态与失败用例

下表冻结目标内部错误码及语义；当前代码未全部实现。HTTP/gRPC 继续由统一错误桥接映射，不在插件或 Skill 中另造状态体系。鉴权中间件的 401/403 保持现有语义；缺对象与无权限使用同一脱敏错误。

| 场景 | 目标错误/处理 | 节点和副作用要求 |
| --- | --- | --- |
| 旧依赖或响应版本进入新执行链 | `skill.tool_contract_upgrade_required` / `agent.response_contract_upgrade_required` | 拒绝；给出所需版本与重新发布动作，不翻译旧结果。 |
| 依赖对象/工具不存在 | `skill.tool_dependency_unavailable` | dependency_check failed；零次工具调用。 |
| key 重复、UUID/key 不对应或声明非法 | `skill.tool_dependency_invalid` | dependency_check failed；零次工具调用。 |
| 精确版本、Schema/实现摘要不一致 | `skill.tool_dependency_version_or_contract_mismatch` | dependency_check failed；禁止改用 latest。 |
| 缺实时授权、跨租户/伪造对象引用 | `skill.tool_dependency_denied` / `evidence.source_access_denied` | permission/source_resolution failed；脱敏，不泄露对象存在性。 |
| protocol/executor 未装配 | `skill.tool_protocol_unavailable` / `skill.executor_unavailable` | dependency_check failed；不改用 LLM 或其它协议。 |
| Pointer/quote/literal/快照不匹配或非法状态提升 | `evidence.source_invalid` | source_resolution failed；不发起该计算。 |
| 非发布公式、业务数值常量或变量绑定非法 | `evidence.calculation_plan_invalid` | input_validation failed；不能让模型临时改公式。 |
| 只有百分比、缺原始计数 | 业务 `needs_action` 或 awaiting_params | 保留 reported 和缺口；该公式零次调用，不是系统执行错误。 |
| 单位或 scope 不可比 | 业务缺口/冲突 | 不执行不满足前提的公式；不能改单位或编造新口径。 |
| 除零/运算超限 | `evidence.calculation_failed`，受保护详情保存原始工具原因 | tool_invoke failed；无 succeeded 回执/计算值。 |
| 超时、取消 | `skill.tool_timeout` / `skill.tool_cancelled` | 失败回执，task failed；取消暂映射 failed 并保留 error code，不添加私有 UI 枚举。 |
| 调用中版本漂移、撤权 | `skill.tool_dependency_version_or_contract_mismatch` / `skill.tool_dependency_denied` | 有失败节点；不能把 adapter 的 HTTP 200 当成功证据。对已发生副作用如实记录。 |
| 输出 Schema 或结果 Pointer 不合法 | `evidence.tool_result_invalid` | result_validation failed；不生成 computed。 |
| 伪造/篡改/跨 run 回执 | `evidence.tool_receipt_untrusted` | evidence_validation failed；不以重算数值相等替代来源验真。 |
| 最终 V5 结构、引用、outcome 不合法 | `agent.response_contract_invalid` | final_validation failed；不回退 Markdown，不持久化为成功消息。 |
| 旧 Registry-only 调用入口 | `skill.legacy_invoke_upgrade_required` | 拒绝且零次执行，提示迁移到发布 Definition Revision。 |

实时 `agent_run.task_failed`、Trace FailNode、历史 snapshot 必须保留同一 error code/stage；业务待补只使用既有 awaiting_params/needs_action，不把二者混为系统失败。重试要新 attempt/执行 UUID、重新授权且保持原固定版本；非幂等有副作用工具不得盲重试，按既有正式合同的幂等策略执行。总体调度仍归 031，不在本任务增加私有调度器。

机器可读用例分两类：schema_cases 验证结构拒绝；runtime_cases 描述必须在 T200/T202/T203/T205 执行的语义与副作用断言。语义反例即使 Schema 合法也必须失败，离线验证脚本明确不把它们记为生产验证通过。

## 8. 旧入口、旧发布与历史迁移

1. **保持现状可诊断**：T199 只冻结合同，当前 tools/v1 + V4 内置计算不会因提交文档改变运行行为。新版本启用前，旧 Runtime 对 tools/v2/V5 必须明确拒绝，不通过放宽字段检查暂时接受。
2. **显式形成新 Revision**：使用受信任目录解析 UUID、精确版本、Schema/实现摘要与权限，更新依赖及 response_contract=V5，重新生成包和 checksum，人工发布。不能修改原发布 Revision、历史报告、运行记录或把插件 Registry 的 published 标志当作新发布验收。
3. **统一运行权威**：Agent 主路径、tenant/skills HTTP、tenant/invocations skill adapter、SkillInvokeService gRPC、Workflow Skill node 统一调用 DefinitionInvokeService 及同一授权/依赖/证据链。未迁移端口显式返回 legacy_invoke_upgrade_required；不能恢复已删除的 PluginSkillHTTPExecutor 或临时注入旧 Registry executor。
4. **禁止重解释 version**：旧 Registry SemVer 不能自动解释为 Revision UUID。旧请求形状如果能无歧义解析治理态 Skill，则省略 version 只解析一次当前发布 Revision 并固定；显式 version 必须是该服务合同规定的当前 Revision UUID，不匹配明确拒绝。旧 SemVer 请求统一给出迁移错误，不能“忽略 version 后运行最新”。skill_id 是保留的机器 key，解析到租户 Draft UUID 后校验当前绑定；不把显示名用于查找。若以后引入独立 revision_uuid 字段，必须同步 HTTP 与权威 Proto，不挪用现有字段静默改变含义。
5. **新链切换门槛**：T200 与 T202/T203 完成前不全局切换；先用明确选择新 Revision 的自建非营销 Skill 验证。启用目标链后，旧 tools/v1/V4 的可执行定义需要显式重新发布；旧 Registry-only 数据保留为目录/安装来源，不能成为运行 fallback。停用旧链和拒绝信息须在发布说明中列出，不能悄悄中断仍在运行的用户任务。
6. **历史只读**：按原 schema 选择对应历史渲染器，明确旧格式/证据范围。V3/V4 不伪造新 UUID、权限决定或回执，不自动转成 V5；需要新证据必须新建 Run。新协议未完整渲染时不启用写入。
7. **无数据库与 seed 隐式动作**：当前 JSONB 可保存新嵌套合同不等于已完成接线，也不要求 T199 执行 migrate/seed。后续确需新持久化模型时独立增加集中迁移及回滚；发布业务 Skill 与部署解释器分别操作。

FR-079 任务选择同时收敛：候选范围来自当前租户、Agent/Team 绑定及权限；意图信息来自已发布的 description/intent_examples、既有团队声明图和用户显式选择，由通用 Planner/Tool-Calling 消费。删除营销关键词/Skill ID 特例，不以新的隐藏业务 key 或确定性白名单替代。不在本轮发明未定义的路由 DSL；相同定义改名、客户自建 Skill 必须使用同一路径。该整改与正式迁移测试归 T203/T204。

## 9. 验证与后续交付

从仓库根目录执行离线合同校验（依赖只进入 uv 临时环境，不修改项目依赖）：

```sh
uv run --no-project --with 'jsonschema[format]==4.23.0' python specs/024-ai-engineering-skills/contracts/validate_tool_evidence.py
```

T199 完成标准是 Schema 自身有效、正反结构样例通过预期、语义失败矩阵和迁移规则明确。它不证明真实工具、生产授权、ledger、历史或浏览器已接线。下一任务 T200 先实现一个 DI 依赖服务并覆盖发布/绑定/执行三个生产调用点；T202/T203 完成可信回执与 V5，T204/T205 再给出真实插件、自建 Skill、变化数据和刷新回读证据。

2026-10-14 离线验证：Schema 自检、全部本地 `$ref`、Python 语法以及 62 个结构正反用例通过；41 个语义用例只检查结构和声明完整性，尚未执行运行时断言。文档回链通过；没有运行服务、数据库迁移或真实工具调用。
