# T202：来源、输入状态与真实工具结果核验

日期：2026-10-14。实现 [T199 合同](contracts/tool-evidence-contract.md) 的可信执行基础层；V5 Runtime、locale、Trace/历史消费由 T203 接线，实际安装插件和浏览器验收由 T204/T205 完成。不能把本地 HTTP 替身视为生产插件调用。

当前状态以 [T203 实现记录](runtime-v5-implementation.md) 为准：新 Revision 的 V5 内置链及受控历史已接线，部署须先执行新增证据表迁移；未修改现有 Revision。T203 还修正了本页第 2 节整份可执行文件摘要的选择，改为实际内嵌工具源包与工具链版本摘要，解决 database/app/Worker 不同入口的标识不一致。下文保留 T202 交付时的检查点。

## 1. 已实现的边界

| 环节 | 实现与拒绝条件 |
| --- | --- |
| 固定声明 | `ToolRunPolicy` 由调用方从固定发布 Revision 编译；冻结依赖、字段、公式、变量字段绑定、精度、比例及结果单位。模型不能更换工具、公式、参数或来源字段。三阶段预检与执行共用 V2 DTO 和依赖校验。 |
| 当前输入 | 创建独立 `powerx.run_input` 对象 UUID 和不可变 JSON 快照，不伪造聊天消息 UUID。任何模型回调前冻结输入，回调只持有深拷贝。非法 UTF-8、过深嵌套、超限输入拒绝。JSON 数值使用 `json.Number`，不经浮点再提取金额。 |
| 授权对象 | 仅允许 DI 按固定 `object_type` 登记 `AuthorizedObjectSource`。reader 必须校验当前主体、租户、对象及所请求字段，并返回最小 DTO 与正式字段元数据；本层再次核对租户/类型/对象/字段。不存在、未登记、跨租户与无权限统一返回 `evidence.source_access_denied`。没有 URL/文件/SQL 或全表对象读取 fallback。 |
| 来源证据 | 来源和 datum UUID 由平台创建。校验快照摘要、严格 JSON Pointer（含数组及转义）、连续 quote、UTF-8 字节范围、完整数字/单位边界与词面值。value 由平台解析；正号、千分位、前导零保留在 literal，value 规范成有限十进制。复用记录须与私有 ledger 完全一致。 |
| 输入状态 | 当前输入为 `user_supplied_unverified` 或 `extracted_unverified`；正式结构化授权字段为 `authorized_record`，对授权文本的抽取仍为 `extracted_unverified`；实际工具输出为 `tool_result`。状态不可由模型授予，计算回执保留原输入状态。授权读取和实际执行都不证明业务事实真实。 |
| 数量、单位、scope | 原文百分比只作 reported，不可截去百分号、改为人数或作为 quantity 参与计算。操作数必须来自已解析 quantity，重复 value/unit/scope 必须逐项相同；跨单位/口径公式不执行。缺业务前提的 needs_action/awaiting_params 投影由 T203 在调用之前完成。 |
| 真实执行 | `ToolRun.Execute` 封存实际请求字节，完成调用后校验正式输出及实时固定版本/权限，再由 Core 创建回执。前置拒绝不生成虚假调用回执；除零、超时、取消、输出非法、调用中漂移/撤权保留失败回执，无成功 result，不自动重试或降级。 |
| 输出引用 | 回执 UUID、租户、Revision、run/trace、工具/Schema/实现、授权决定、完整输入及产物摘要必须与同次私有 ledger 一致。只可引用 adapter 正式声明的数值输出 Pointer，实际标量、单位和 scope 必须一致；不能引用 precision 等元数据冒充结果。历史/子 run 回执不能导入当前 ledger。 |
| 再次消费 | `VerifyResultReference`、工具输出来源解析和 `VerifyDatum` 会重查当前工具绑定及原来源权限；不能用旧 authorization 决定恢复权限。`VerifyReceipt` 只证明执行事实，不授予访问权。 |

核心代码位于 [tool_contract.go](../../backend/pkg/corex/agent/evidence/tool_contract.go)、[tool_sources.go](../../backend/pkg/corex/agent/evidence/tool_sources.go) 和 [tool_runtime.go](../../backend/pkg/corex/agent/evidence/tool_runtime.go)。新协议完整对象上限 1 MiB；每 run 最多 512 datum、64 次实际调用，私有字节预算 64 MiB；调用前预留请求/最大输出/回执空间。adapter 还须在传输读取时执行上限和 deadline，不能等全部下载后才截断。

## 2. typed 计算与生产开放状态

`powerx.calculation.evaluate@2.0.0` 已用 typed request/result 调用现有受限 AST/有理数计算器。固定 `half_away_from_zero`，先按 percent 乘一百再舍入，value 不携带单位，保留 precision；不改变旧 evaluate@1.0.0 与 V4 wire 格式。

内置描述由 `ToolService.CalculatorDependency` 生成：Schema 摘要针对随 Core 发布的完整合同原件，实现摘要针对 Core 可执行文件字节，绝不使用版本标签或 provider 自报摘要。部署必须以不可变发布文件启动 Core，不能运行中原位覆盖该可执行文件；Core 构建更新会改变实现摘要，使用新链的 Revision 必须显式更新固定描述并重新发布。描述不是用户/模型自由填写的配置。

[生产 factory](../../backend/internal/service/skills/dependency_runtime.go) 已可检查真实内置描述；伪造摘要报版本/合同不符。**V5 消费仍未开放**，即使内置描述正确，当前 Manifest 的 V2 发布/绑定/执行仍明确不可用，不借用 V4 消费新协议。正式外部 pinning adapter、service_actor 授权和业务对象 resolver 均未在生产登记，继续失败关闭；T202 提供这些端口的校验/封存边界，没有把旧任意 capability 调用包装成可信新回执。

现有 V4 已实际应用输入冻结/回调隔离及严格 JSON 解码（拒绝重复字段/非法 UTF-8），保留原响应 Schema、数值及历史语义；不会伪造新 UUID/回执迁移旧数据。

## 3. T203 接线要求

1. 从真正固定的发布 Revision 编译 `ToolRunPolicy`，传入服务端 run/trace/Revision/租户及可信 reqctx；禁止模型/context map 自报身份。新 run 固定当前主体，换用户、租户、member 或服务凭据不能复用。
2. 新链所有计算通过 `ResolveDatum → Execute → VerifyResultReference`。不能只用 `VerifyReceipt`、重新哈希或重新算出相同结果替代来源/权限核验。已有 calculation_policy 的动态字段/缺参必须经过可信策略编译，不能直接把模型选择当成发布规则。
3. `BoundTool` 是可信 DI 端口，必须固定实际实现、合同/Schema、registration/adapter，实际执行并校验请求/输出和来源绑定，执行后/再次消费时重新授权。provider 只返回自身正式输出，不能填受信任元数据；不得给默认总是成功的校验回调。
4. 当前快照和请求/输出仅保存在每次运行的私有内存 ledger。T203 必须将实际冻结字节及回执落入受控历史存储，按原租户/对象权限读取并校验完整性。`ReadArtifact` 只供受信任平台保存执行事实，不是客户端授权接口；历史禁止导入当前 ledger、重算或用当前对象补旧快照。历史存储/清理前不开放 V5 写入。
5. 取消后的失败回执需用保留同一可信主体的清理上下文保存（例如 `context.WithoutCancel`），不得重试工具。最终错误统一保留 code/stage 到失败节点、Trace、实时及历史，不伪装业务待补。

## 4. 本地回归及验证范围

新增 Go 测试执行了真实内置计算；外部工具使用 `httptest` HTTP 上游和明确的授权/pinning 替身，授权对象使用 typed reader 替身，**没有安装环境或真实业务仓储读取证明**。回归覆盖：

- 30/12、百分比、正负半舍入与负零；冻结后调用方/模型修改 map 不影响实际输入。
- 只有百分比、虚构人数、伪造 UUID/quote/范围/Pointer、非法 UTF-8、状态提升与来源快照损坏。
- 对象/字段/租户拒绝与对象不存在的相同脱敏响应；冻结版本不随对象更新；来源撤权后拒绝消费派生结果。
- 未发布公式、业务数值常量、重复值、工具/Schema/实现漂移、单位/口径混用在调用前拒绝；不产生回执。
- HTTP 实际成功、非法/重复输出、调用中版本漂移与撤权、超时、取消；失败有回执且无 result。
- 回执字段/产物字节篡改、跨 run 重放、换主体、额外输出层级、将元数据当结果、修改结果单位/口径；并发回执隔离及 race 检查。

从 `backend` 执行：

```sh
go test -race ./pkg/corex/agent/evidence ./internal/service/skills -count=1
go test ./pkg/corex/agent/evidence -v -count=1 > /tmp/powerx-t202-contract.log
```

从仓库根执行：

```sh
uv run --no-project --with 'jsonschema[format]==4.23.0' python specs/024-ai-engineering-skills/contracts/validate_tool_evidence.py --go-test-log /tmp/powerx-t202-contract.log
```

离线 Schema 自检及 62 个结构用例通过，17 个 Go 实际 DTO 样本与冻结 Schema 一致。原 41 项 runtime_cases 仍只由该 Python 脚本检查结构/声明；具体运行语义由 Go 测试验证，**不能宣称全部端到端语义用例已通过**。真实插件、统一 Runtime/失败节点、浏览器及刷新回读仍按 T203～T205 验收。

skills、agent、agent_authz、bootstrap、utils、evidence、seed 七个相关包的测试通过；evidence/skills race 检查和相关静态检查通过，Core `./cmd/app` 构建通过，临时构建产物已清理。未运行全仓测试；旧 Skills integration 包引用已删除 executor 的构建缺口仍见 [T200 记录](dependency-preflight-implementation.md)，本轮不恢复旧桥接器。验证只使用本地夹具，没有数据库迁移、seed、实际 grant 或用户服务操作；测试 HTTP 上游随测试退出清理。
