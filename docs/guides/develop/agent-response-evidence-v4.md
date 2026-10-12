# V4 计算证据报告：实现与迁移

状态：2026-09-08，代码与定向测试已实现，已开展已发布 Skill 的真实 Ollama 验证；完整团队、插件工具和历史 UI 联调仍需独立验收。不能仅因本页存在就宣称 Phase 23 已全部交付。

## 执行边界

Skill 通过 `executor.calculation_policy` 声明输入字段、公式和口径。`output_mode=response_envelope` 的平台执行链路分为通用原文事实抽取、声明字段映射、声明式计划、工具执行、说明生成与报告封装。模型不能临时创造公式、倒置分子分母或修改百分比尺度。Core 的 `pkg/corex/agent/evidence` 解释配置并执行通用计算，最终生成 `powerx.agent.response/v4`。这里没有营销 Skill/Agent/Team 标识分支。

1. 平台从明确声明的 evidence_sources 生成通用原文数值片段。模型先提交 `powerx.agent.evidence-facts/v2`，facts 为平台 token key 索引的对象，每个可命名 token 都列入 required；每项只有受原文枚举约束的 label 和 scope，禁止省略已提供的事实。平台从 token 回填数值和单位，漏项或未声明项明确失败。该层保留任意明确数值业务陈述为 `reported`，但不授予计算资格。
2. 平台再按 policy.unit_tokens、字段证据词和活动画像生成受控候选片段。模型提交 `powerx.agent.evidence-source/v1`（schema、data），其中 data 是以 policy.input_fields 的字段 key 为键的对象；每个值只选择 token_ref 和 scope，不能填写或修改数字、单位。
3. 平台按已发布的 calculation_policy 生成内部计划。公式、bindings、precision、percent、compare_to 来自 Skill Revision，不由模型生成。只有触发条件成立且操作数齐全时执行；缺操作数进入明确缺口，禁止反造人数。
4. 平台组装内部 `response-draft/v1` 调用纯计算工具，产生数值、冲突和真实凭证。模型不提交 display_value。
5. 模型单独提交 `powerx.agent.evidence-notes/v1`（schema、hypotheses、gaps、actions）。说明阶段不接受操作数、数值或凭证字段，不重跑计算。每个数组最多六条、每条最多三百字符，数值统一引用表格中的指标名称，不在自由说明中再次计算。
6. 平台验证说明并封装 V4；任一阶段错误直接失败，错误带 facts/source/plan/calculate/notes 阶段，不自动修补、不切换旧草稿或自由文本模式。

三个模型阶段均复用统一 AI Service 的单次请求超时，未新增整轮超时。测试程序的总时限不等于生产 Runtime 超时。阶段提示词位于 `backend/internal/service/skills/locales/evidence.*.json`；业务公式、声明字段名称及范围来自已发布 Skill 的 calculation_policy，不从提示词自由文本解析或推断。未声明的通用事实 label 仍只能选择原文词面。

- 模型的 `data` 以业务 key 为唯一键，值只包含 `scope/token_ref`；key 必须来自 policy.input_fields，token_ref 必须指向单位符合该字段声明的原文片段。对象结构在协议层禁止同一字段重复选择；未知 key、未知 token_ref 或单位不匹配均明确失败，并在受保护的执行追踪中记录阶段与已选映射。`label/kind` 来自 Skill；`unit/source` 来自平台数值片段。source 包含声明输入的 JSON pointer、精确引用 quote 和纯数字词面值 literal。原文数字、分组逗号、数字与单位间空白均保留，不把“万元”自行换成“元”。这是主流程的原文取证，不是从模型自由文本解析结构化结果的兜底。
- 单个声明来源最多 128 KiB，总数值片段最多 512 个；未知单位不猜成无单位数量。支持新的单位须更新 Skill 的 unit_tokens，不能静默换算或丢弃单位后计算。字段映射和 scope 仍包含模型判断，不等于已独立核实业务口径。
- 团队编排把原始用户材料写入每个 Skill 的 `payload.message`；使用 `/message` 的 evidence_sources 因而在直接执行和团队汇总中含义相同。`payload.content` 是材料载体，不是来源路径的兼容别名。自建 Skill 若声明其他来源路径，编排方必须按其已发布契约提供该字段，否则以 `evidence.source_missing` 明确失败。
- 说明模型收到指标名称、已执行计算的操作数名称及对照状态、冲突和缺失字段，以及已发布 Skill 的业务要求。声明来源的原文和显式 upstream_* 意见以 source_context 传入，数字统一屏蔽；原文标记 original_source，上游标记 upstream_opinion，均明确未经独立核实。上游只能辅助综合解释，不能成为计算来源、覆盖原文或修改缺口。缺失声明来源和超过 128 KiB 的说明上下文明确失败，不截断来伪造完整性。最终 V4 仍保存完整数字与凭证。
- `kind=quantity` 表示声明的原始数量或金额字段，提取值仍未经独立核实；`reported` 表示原文已报告比率。原文只有百分比，不能制造分子和分母。
- `calculations`：`key/label/expression/bindings/precision/percent/compare_to` 全部必填。变量绑定到 data.key；报告公式不能出现数字常量。比例尺度由 percent 显式指定。
- 同一表达式要求操作数声明相同 `scope` 和 `unit`，不做隐式单位换算。scope 的业务正确性仍须核实，字符串相同不证明归因成立。
- `hypotheses/gaps/actions`：业务假设、缺参和行动；不得包含阿拉伯数字，所有数值放入原文或计算表，防止在自由文字中重复心算出另一个结果。不允许把原文主张改称已验证事实。

## 工具与结果

本轮内置纯计算工具 key 为 `powerx.calculation.evaluate`，版本 `1.0.0`。支持 `+ - * /`、括号和显式变量，采用有理数运算，最终按指定小数位四舍五入；百分比先乘 100 再舍入。不支持函数调用、属性访问、任意代码或网络访问。

限制：表达式 512 字节，最多 32 变量、128 步，精度 0～12，数值词面值 80 字节，中间有理数 4096 bit。取消、除零及超限均显式失败。

最终报告必须包含 `schema/kind/outcome/presentation/source_digest/tenant_uuid/revision_uuid/trace_id`。

| presentation 字段 | 来源与展示 |
| --- | --- |
| reported | 精确引用原文的提取值，统一标记为未核实 |
| computed | 平台执行记录：UUID、工具版本、操作数、表达式、精度、时间、真实计算值 |
| conflicts | compare_to 指向的原文值与工具结果不一致，保留两者 |
| hypotheses、gaps、actions | 假设、待补信息与下一步 |

存在冲突、缺口或假设时业务 outcome 为 `needs_action`；它不是工具执行失败。结构错误、伪造来源、未执行结果、工具错误则为系统失败。Runtime 还要验证报告确实来自同次执行的 ledger，不能仅验证 JSON 格式。

权威结构为 [V4 JSON Schema](../../../backend/config/agent/response_envelope.v4.schema.json)，由 Go Report 类型导出并通过测试核对。模型阶段 Schema 由 `SelectionJSONSchema` 和 `NotesJSONSchema` 产生。`DraftJSONSchema` 描述内部工具输入；不接受模型一次提交完整草稿。Web Admin 从 locale 统一生成章节与表格，历史回读使用同一渲染函数。

## Skill 中的声明式计算策略

`executor.calculation_policy.schema=powerx.skill-calculation-policy/v2`，严格要求：

- `activity_profiles`：每项 `key/label_i18n/evidence_any_i18n`。平台只在声明来源中按这些词面确定适用业务类型；模型不能把交易/留存材料改判为线索活动。无 profile 命中时不套用最近似模板、不产生声明公式；已抽取的通用原文事实仍保留。
- `input_fields`：每项 `key/kind/unit_tokens/label_i18n/description_i18n/evidence_terms_i18n/applies_to`。`evidence_terms_i18n` 是字段的原文上下文约束；候选 token 必须同时符合单位与字段词面，`applies_to` 必须指向已激活 profile。单位相同不代表业务语义相同，例如“6个月”不能成为“6个访问”。
- `formulas`：每项 `key/label_i18n/expression/bindings/precision/percent/compare_to/when_any_present/applies_to` 全部必填。公式只在其 `applies_to` 与当前 profile 相交时才会进入计划。bindings 从表达式变量映射到 quantity 字段；compare_to 是 reported 字段或空字符串。
- `when_any_present`：任何所列字段在**同一适用 profile 的合格证据**中存在，才考虑该公式；全部不存在表示该公式不适用于这次输入，不引入无关缺口。条件成立但缺少绑定操作数时不执行，交给说明阶段明确补数；说明完全忽略缺口会失败。说明阶段的 gaps 只能从平台按 missing_inputs 生成的 allowed_gaps 中选择，并覆盖全部缺口；没有缺失操作数时 gaps 必须为空，不能把已提供数据或定义核实事项改写为补数要求。
- 发布、可运行绑定和执行前校验 profile、字段、公式、类型、公式白名单、精度和 locale。V1 或缺少以上字段的策略明确报 `skill.calculation_policy_invalid`；必须升级 Skill Revision 后重新发布，不能退回模型自由编公式。

该策略是 **PowerX 执行扩展**，不是把业务写入 Core：`SKILL.md` 仍可作为 Claude Code、Codex 等生态共同可读的包核心；可执行的 PowerX 能力放在 `powerx/manifest.json` 的 executor 扩展中。Core 只解释这个公开、版本化的声明，不识别营销、团队或 Agent 标识。外部仅含 `SKILL.md` 的包可作为 `instruction_only` Draft 导入；未补全 PowerX executor、权限与策略前不得执行。

固有营销策略源文件为 [marketing_calculation_policy.json](../../../backend/cmd/database/seed/locales/marketing_calculation_policy.json)，seed 将其写入数据库的 Skill Revision。文件只声明公式与字段，不保存本次活动的金额、百分比或计算答案。客户的策略同样随自己的 Skill Definition 发布，不要求修改此 seed 文件。当前演示覆盖策略明确列出的字段和公式，不代表已支持任意渠道分组、跨单位成本指标或所有营销指标。

## 依赖与尚未完成的边界

`executor.model_policy.parameters` 可显式声明 `thinking`（布尔）、`max_tokens`（1～16384 整数）。其他参数拒绝，未声明不注入默认值。Bootstrap 将这些参数交给现有统一 AI Service，单次 LLM 超时不变。营销汇总种子明确使用 `thinking=false/max_tokens=4096`，用于原文提取和算式提交，不全局关闭其他 Agent 的思考或修改 AI Settings 温度。

`tool_dependencies.schema=powerx.skill-tools/v1`，每项必须声明 `tool_key/version/input_schema/output_schema/permission`。本轮计算工具使用 `response-draft/v1` 与 `response/v4` 全名作为输入输出契约，permission 为 `pure_calculation`，不包含外部数据读取授权。

发布、Agent 可运行绑定、执行前已调用公共检查函数。内置计算工具要求精确版本与契约；缺依赖阻断。外部依赖要求受信任 checker，未接入时明确报 `skill.tool_dependency_unavailable`。

**仍未交付**：外部 checker 与生产 Capability Registry/租户授权/固定协议执行的完整接线、客户自建插件工具的真实调用验证。现有 Capability executor 仍有固定 `core_internal` 选择，workflow executor 仍返回 unavailable，不能声称任意插件或脚本现在都能执行。不得通过放宽检查绕过这些缺项。

## 迁移与测试

1. 本次没有新增数据库字段，不需要为本修改执行 migrate；migrate 与 seed 仍独立。
2. 重新发布 Skill：最终 llm_prompt executor 声明 `output_mode=response_envelope`、`response_contract=powerx.agent.response/v4`、`evidence_sources`、V2 `calculation_policy`，并声明工具依赖。固有营销种子定义已更新，需要成功运行 `make seed` 才进入数据库；客户定义必须在页面或发布 API 中形成新的 Revision，不能依赖旧 Revision 自动迁移。
3. 重启后端并更新 Web Admin，创建新任务验证。旧 V3 不自动转换为 V4；历史界面明确提示契约升级，保留原始数据供诊断，不伪造新执行凭证。
4. 验证原文 29、34.2、3.37：在同口径声明成立时，按声明精度计算；营销策略为两位小数，输出 0.85，并与原文 3.37 分列为冲突。只有 27.8% 时必须保留报告值，不得产生人数。
5. 同时更换数值和 Skill key；验证来源截断、跨输入引用、不同单位/口径、除零、缺依赖与其他运行凭证被拒绝。特别验证“6个月”等时间量词不能进入访问、线索或客户数候选；GMV/ROI/复购材料不得激活线索目标公式。

定向验证：`go test ./pkg/corex/agent/evidence ./internal/service/skills ./cmd/database/seed ./internal/service/agent`；前端 `npx vitest run tests/unit/agent/response-envelope.spec.ts tests/unit/agent/history-message-meta.spec.ts`。真实模型成功、SSE 和历史 UI 回读仍需单独记录运行证据，单元测试不替代发布验收。

### 2026-09-08 实测记录（汇总 Skill 通过，团队端到端待验收）

- `make seed` 已成功，当前验证 Revision 为 `a6f795f6-7d1d-45a2-b40e-be713de45043`，包含 unit_tokens 声明，使用本地文件存储。没有把 migrate 加入 seed。
- 显式启用 `TestPublishedEvidenceLive`，从 PostgreSQL 读取已发布 Skill，经统一 AI Service 调用本机 Ollama/qwen3:8b。该测试不创建完整团队 Run，不替代 HTTP/SSE、Trace 界面或历史恢复测试。
- 当前原始数据用例通过，耗时约 15.77 秒：34.2 万元投入、46.2 万元 GMV、29.0 万元增量 GMV，断言计算结果为 1.35 和 0.85；0.85 与原文 3.37 保留冲突；27.8% 只作为原文值，没有编造人数。测试 trace 为 `a2ae63eb-dbaa-4e25-9e13-da1670991a7a`。
- 当前替换数据用例通过，耗时约 22.82 秒：20 万元投入、40 万元 GMV、10 万元增量 GMV，断言结果为 2 和 0.5；0.5 与原文 1.5 保留冲突；15% 只作为原文值。测试 trace 为 `70bea993-d6e3-4aec-bea7-9791bc0b6aef`。这两条是测试调用的 trace，不是已持久化的团队 Agent Run。
- 以上验证覆盖结构、原文数值、工具计算、冲突和凭证。说明模型仍可能给出泛化或多余的核实建议，不能据此认定完整业务解释已验收；原文归因真实性也未独立验证。
- 以下为修复前失败记录，保留用于说明回归目标：
- 一轮约 58 秒返回，模型把原文 reported ROI 作为单变量计算的操作数，被 `evidence.quantity_reference_required` 拒绝。已增加对应回归测试；不能允许身份复制把原文主张升级为工具计算证据。
- 补充提交检查后，一轮约 53 秒返回，原始金额和两条除法算式符合要求，但模型仍在 hypotheses/actions 内自行输出数值与公式，被 `evidence.numeric_narrative_forbidden` 拒绝。该轮也没有完成业务验收，不能记为成功。
- 更早一轮 provider 返回 HTTP 500 `peg-native` 格式错误，另有一轮持续生成直到 300 秒取消。JSON Schema 目前只约束结构，严格语义校验仍由 Core 执行；不通过截取 JSON、放松数值校验或模型自动修补绕过失败。
- 定向后端测试、前端 5 项测试、前端构建已通过。Runtime 全包曾在 `TestEngineInitialTaskExtractsRequiredSlotsFromUserMessage` 失败，但该测试单独连续 5 次通过；全包不能标为全绿，需另行定位共享状态/顺序影响。

下一验收门槛是更多真实输入的字段与口径判断、完整团队真实运行、刷新后内容与状态一致，以及外部插件工具生产接线。当前两组成功不证明这些剩余门槛完成。

## 2026-10-06 事实覆盖率回归

营销 SaaS 原材料已保存为 `backend/tests/fixtures/agent_runtime/marketing_review_saas.txt`。事实 v2 的 required 对象覆盖全部可命名数值；计算字段同样要求覆盖 Schema 中所有合格候选字段。字段匹配只看数值所在短句，按 Skill 声明的 evidence_terms 长词优先，区分 GMV 与增量 GMV，避免同邻近上下文里的金额串位。数值 token 保留词面位置，去重区分不同来源短句，不能合并两个渠道的同值陈述。范围上下界保留原单位，日期/编号不成为指标，通用 reported 仍不获得计算资格。

原 Run `b8612aff-646c-4e55-acba-3da8e15d49f3` 的已发布 calculation_policy 与种子一致；本次不修改已发布 Skill Revision 或历史报告。新内部事实阶段明确使用 v2，不能静默接受 v1 子集输出。回归要求保留 GMV、ROI、增量 GMV、复购率、渠道 ROI、点击率、下单转化率及客单价上下界；按已声明规则复算两项比值，识别一处复算口径冲突，真实缺口为复购率复算所需原始客户数量。它不独立验证原文增量归因。

可选真实 Ollama 检查（读取私有配置、共享当前 Redis 物理模型池，不写业务记录、不重放历史 Run）：

```sh
(cd backend && POWERX_TEST_EVIDENCE_RUNTIME_CONFIG="$PWD/etc/config.yaml" go test ./internal/service/skills -run '^TestRealOllamaEvidenceSaaSReview$' -count=1 -v)
```

该检查使用已发布模型参数一致的非思考模式和 4096 输出上限，温度显式设为零以检查契约。它证明真实模型能完成新协议，不代替重启正式后台后页面重试的端到端验收。


## 2026-10-07 字段语义与综合解释修复

`calculation_policy/v2.input_fields` 新增两个可选声明，旧策略无需补字段：

| 字段 | 行为 |
| --- | --- |
| `scope_i18n` | 按字段语言声明固定业务范围，必须覆盖字段语言且非空。选择 Schema 使用 const，字段选择和计划再次校验。固定标签不等于客户池或归因已经核实。 |
| `token_role` | range_start 或 range_end，只匹配相邻数字之间明确的单位及范围连接符，分别保留上下界原单位。不把“达到”视为范围，不自行换算；其他值明确失败。 |

营销字段名称和范围作为内置 Skill 发布数据维护：客单价上下界、未续费/未升级回溯期、活动周期、复购观察期、短信及信息流指标有独立 key。它们为 reported，不因命名更准确而获得计算资格。复购人数、客户池及报告率采用相同声明范围，实际周期和群体仍须一致。

说明阶段 calculation_checks 区分 matched、different_under_declared_formula、not_compared。已执行计算的操作数不能再被称作缺失；对照一致无需重复核对算术。公式下不一致仅触发口径核实，不自动判定原文错误。渠道比率差异本身不构成计算冲突。原文业务目标和归因解释须保留为原文陈述或待验证判断。

本修改不变更 V4 响应字段、平台 Capability 或数据库结构。发布只创建新 Skill Revision，历史 Run 与旧 Revision 保持不变。定向发布在 backend 目录执行：

```sh
go run ./cmd/database seed-native-marketing-skills --config etc/config.yaml
```

命令只更新内置营销 Skill，不重置 Agent、团队或其他模块种子。加载新解释器需重启后台后创建新 Run；不要改写历史报告来伪造修复验收。

回归矩阵：原案例所有数值保留、范围上下界不串位、时间不变人数、固定范围拒绝篡改、未命中 profile 仍保留通用事实、数字叙述及虚构缺口拒绝、上游错误不覆盖已有操作数、原文和上游说明数字屏蔽、计算凭证及 V4 历史展示一致。真实模型检查读取已发布 Revision，保存独立 Trace 与结果；完整团队 HTTP/SSE 仍需新 Run 证据。


### 来源条件约束的业务说明

`executor.review_policy` 是可选的公开 Skill 扩展，schema 为 `powerx.skill-review-policy/v1`。未声明时沿用通用说明生成；声明后，hypotheses/actions 只允许本次规则命中的文字，漏项、重复或新增说明明确失败，不依赖提示词保证。

规则字段：key、target（hypotheses/actions）、text_i18n、evidence_all_i18n、when_any_fields、when_any_conflicts、when_any_missing；可选 when_any_computed。各非空条件组之间为 AND，组内字段/计算 key 为 ANY；原文词面必须在同一个声明来源内全部命中。字段指向 calculation_policy.input_fields，计算引用指向其 formulas；发布、绑定和调用均校验引用、语言和说明长度，文字不得含数值。空条件组不约束，但规则不能全部无条件。输入数字和上游意见不能触发“已经独立验证”的说明。

when_any_fields 检查实际报告值，when_any_computed 检查真实已执行计算，when_any_conflicts 检查真实复算冲突，when_any_missing 检查计划给出的缺失操作数。原文条件只查看 evidence_sources，不查看上游意见，避免错误意见变成证据。命中规则按声明顺序提供给说明模型，Schema 固定完整枚举和去重数量，Core 再校验。超过每类六条的匹配结果明确报错。

内置营销策略位于 backend/cmd/database/seed/locales/marketing_review_policy.json，包含原文老客激活目标、原文人群匹配主张、产投比与盈利的边界、复购质量的验证边界，以及增量口径、复购人数、人群对照、渠道原始明细四类行动。它不把点击率称为低，不声称复购已经提升，也不要求重复核对已匹配的算术。业务文字与来源条件都发布在 Skill Revision 中，Core 不识别营销字段或角色标识。


### 本次真实模型验收

从 PostgreSQL 读取已发布汇总 Skill，经统一 AI Service 和共享 Redis 物理模型池调用 Ollama/qwen3:8b。输入为 Run 3c61d433-69a5-477b-a310-2bf40a9f97ef 的原文及归档上游意见（包含修复前“已有金额缺失”的错误意见）。独立检查 Trace 6aeb43f0-fc5b-4065-a2a6-ef9fa556b4eb，耗时约 137.33 秒。

结果保留十五项原文数值，准确标注上下界和渠道；工具结果仍为 1.35、0.85，保留一处同声明公式下的口径冲突。最终包含四条受来源/状态约束的业务说明和四项去重行动，复购率缺失人数仍作为复算缺口。没有复制屏蔽标记、凭空称点击率低、宣称复购已提升或重复要求核对已匹配的活动产投比。

本次还修复了种子比较中 typed 工具依赖与 JSONB 字段顺序不一致造成的重复发布。验证 Revision 与当前默认 Revision 的完整 definition_json 相等；修复后重复定向发布不产生新 Revision，历史 Run 回复未改写。具体 UUID、相等检查及内容摘要见 evidence/marketing-review-semantics-20261007.json。

此为已发布 Skill 的真实执行证明；8077 未由代理重启，正式团队新 Run 的 HTTP/SSE/历史页面验收尚需重启后完成。
