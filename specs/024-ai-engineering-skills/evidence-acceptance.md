# T205：计算证据验收记录

日期：2026-10-15。承接 [T204](plugin-tool-example-implementation.md)。本次已完成本地真实模型、真实插件、持久化原件、前端数据链，以及本机 PostgreSQL 迁移、V5 示例发布和真实浏览器验收，T205 完成。远程部署、日常签名配置和其他插件各自的安装升级仍按下面的边界执行。

## 证据分层

| 验证层级 | 本次使用的实际对象 | 能证明的范围 |
| --- | --- | --- |
| 内置工具 | 当前构建的 `powerx.calculation.evaluate@2.0.0`、正式 Schema 和实现源包摘要 | typed 精确计算、来源/结果/ledger 核验 |
| 真实模型 | 已配置物理容量池中的本机 Ollama `qwen3:8b`，两份材料、六次真实模型请求 | 发布 Revision → 真实 facts/source/notes → 两次 typed 计算/报告 → 新 Service 回读；未改写模型输出 |
| 真实插件 | 实际构建的 base 工具二进制，由隔离 Manager/Supervisor 启动固定副本 | 三个 HTTP base、两个客户 Skill、十八次成功计算及一次真实 HTTP 超时请求 |
| 受控历史 | 隔离 SQLite 中真实 `EvidenceSnapshot`，原用户/租户与当前 Agent grant | 原件完整性、刷新投影、撤权拒绝、旧进程停止后只读回显 |
| 前端 | 上述后端即时导出的 24 条 metadata，实际 normalization/renderer 与双语 locale | JSON 刷新后数值/标签/状态保持一致；不是手写报告夹具 |
| 当前安装环境 | 真实 `powerx` PostgreSQL、集中迁移、专用 V5 发布、同源临时 Core | BYTEA 归档表、旧数据保留、幂等发布；日常 JWT 配置保持原样 |
| 浏览器 | 独立 Playwright Chromium 143、真实登录、Ollama `qwen3:8b`、当前已发布 Revision | 真实聊天 → V5 → 刷新后选择历史会话 → Trace 200；不代表远程部署验收 |

前期两组真实模型的 Source/Draft/Publish 使用隔离库，发布包由真实 PackagePublisher 生成。本机安装验收另在明确授权后执行集中迁移和专用示例发布，并创建测试拥有的 Agent/会话；结束后通过正式 API 删除测试对象。物理模型调用遵循已有 Redis 容量池，不绕过共享并发控制。

## 场景与断言

| 场景 | 输入/操作 | 核验结果 |
| --- | --- | --- |
| 插件原值 | 12.5 × 4 × 2 | 100 |
| 插件小数替换 | 0.1 × 0.2 × 0.3 | 0.006；不先转浮点再保存 |
| 插件长整数/小数 | 123456789012345678901234567890.125 × 2 × 1 | 246913578024691357802469135780.25；刷新不丢精度 |
| 内置值变化 | 18 / 4、1 / 3、大整数 / 2 | 4.5、0.33、6172839450617283945 |
| 同口径/等价小数 | 30 / 12，原文 2.50 | 完成；保留原文，不制造数值冲突 |
| 原文与计算冲突 | 30 / 12，原文 3 | needs_action；同时保留原文和真实结果 |
| 跨 scope | 分子/分母 scope 不同 | typed gap；不执行该式、不隐式合并人群 |
| 百分比而无计数 | 原文 27.8%，无分子分母 | 原文保持 reported/%，缺两个操作数；无计算/回执，不生成 27.8 个客户 |
| 除零 | 30 / 0 | failed、`evidence.calculation_failed/tool_invoke`，保留实际失败回执，无成功结果 |
| 缺合同/停用 adapter/版本漂移 | 改隔离目录的治理状态 | 生产预检阻断，真实插件 HTTP 调用数不增加 |
| 伪造来源 | 普通 context 自报来源，或 Planner 数字不同于入口冻结值 | `evidence.source_invalid`，HTTP 前拒绝 |
| 撤权 | 当前 Agent grant 禁用 | 新调用拒绝，历史投影 unavailable；不保留可展示报告/标签 |
| 工具超时 | SIGSTOP 暂停测试拥有的进程，adapter deadline 为 50 ms，结束后 SIGCONT | Manager 发出真实 HTTP；归档 timed_out 回执、`skill.tool_timeout/tool_invoke`、无 result/成功报告 |
| 历史回读 | 旧进程停止，新建 Service | 读取冻结原件与原 Revision 标签，不再调用工具/模型 |
| 模型原始材料 | 34.2 投入、46.2 GMV、29.0 增量 GMV | 实际结果 1.35 / 0.85，与原文增量主张的冲突独立保存 |
| 模型替换材料 | 投入 20.0、GMV 60.0、增量 GMV 10.0 | 实际结果 3 / 0.5，原文报告值仍独立；不是复用上次结果 |

十八次插件成功调用分布在 `/api/v1`、`/api/v2`、`/gateway/custom`，各两个不同名字的客户 Skill，每个 Skill 三组数字。实现/Schema/provider/工具版本和实际输入输出摘要保存在本次真实回执与归档中，不以目录自报摘要冒充实现证明。

超时表示 Core 未取得可确认的成功结果；恢复进程后插件仍可能处理已排队请求，不能把超时说成“绝对没有执行”。该接入只开放无副作用纯计算，测试也不做业务写入。

## 验收发现与修复

真实模型的两组材料最初都通过来源选择，却在 typed 来源解析阶段失败。无模型逐 token 回归复现：`6000元到…`、`2.4万元之间…`、`90天内…` 等合法中文被拒绝。

原因是 tokenizer 允许单位后的中文语法词，ResolveDatum 另写一套字母边界，把中文汉字也视作非法单位后缀。本次统一为 `numericUnitBoundary`：tokenizer 与 typed 来源核验共用规则，允许明确单位后接汉字；继续拒绝百分号截断、`6个月` 截成 `6个`、`12cms` 截成 `12cm`、单位后接数字及隐藏格式字符。没有按营销字段或 Skill ID 添加特例，没有把模型输出当作可信数值。

修复后逐 token、安全反例、V5 同/跨口径矩阵、真实模型两组全部通过。由于内置工具固定实际生产源包，源包变化会改变实现摘要，部署时仍须显式发布当前构建的新 Revision。

旧 Skills 集成夹具按现行合同调整：Registry 没有 executor 时验证升级拒绝及 failed audit；匹配/性能测试仍检查被选择对象与边界；发现夹具改为 capability/prepare/action 声明；候选性能夹具补显式绑定；Trace 夹具补消息归属；SQLite 聊天夹具补 UUID 并隔离/关闭连接。没有增加旧透传或放宽生产授权。

## 复现命令

从 Core 仓库根执行。实际插件源码和私有模型配置缺失时，对应用例会明确 skip；本次二者均实际运行，未用 skip 计通过。

```sh
# 先创建本轮独立证据目录；输出必须是新文件或空文件，防止混入上次原件。原件含测试生成的 UUID，不写入仓库。
T205_DIR=$(mktemp -d /tmp/powerx-t205-evidence.XXXXXX)
export POWERX_TEST_EVIDENCE_EXPORT="$T205_DIR/actual-history.jsonl"

# 真实 native 工具 + 原件导出
POWERX_T204_BASE_ROOT=/path/to/PowerXPlugin go -C backend test -v ./tests/integration/skills -run '^TestT205PinnedBasePluginEvidenceMatrix$' -count=1 > "$T205_DIR/native.log"

# 真实模型 + 已发布 V5 + 新 Service 回读；配置路径相对 skills 包目录，不能复制配置到报告。
POWERX_TEST_EVIDENCE_RUNTIME_CONFIG=../../../etc/config.yaml go -C backend test -v ./internal/service/skills -run '^TestRealOllamaEvidenceSaaSReview$' -count=1 -timeout=15m > "$T205_DIR/model.log"

# 冻结 Schema 核对实际 DTO，不代替上面的执行语义断言
uv run --no-project --with 'jsonschema[format]==4.23.0' python specs/024-ai-engineering-skills/contracts/validate_tool_evidence.py --go-test-log "$T205_DIR/native.log"
uv run --no-project --with 'jsonschema[format]==4.23.0' python specs/024-ai-engineering-skills/contracts/validate_tool_evidence.py --go-test-log "$T205_DIR/model.log"

# 前端实际原件刷新数据链；继承上面的 export，不使用手写报告兜底
(cd web-admin && npx vitest run tests/unit/agent/actual-evidence-refresh.spec.ts tests/unit/agent/response-envelope-v5.spec.ts tests/unit/agent/response-envelope.spec.ts tests/unit/agent/durable-refresh-connection.spec.ts tests/unit/agent/history-message-meta.spec.ts)

go -C backend test ./tests/integration/skills
go -C backend test ./pkg/corex/agent/evidence ./internal/service/skills ./internal/server/agent/runtime ./internal/service/agent_run
go -C backend test -race ./pkg/corex/agent/evidence ./internal/service/skills ./internal/server/agent/runtime ./internal/service/agent_run
```

前端本轮 5 个文件、46 项通过，其中 25 项直接消费真实后端导出的 18 份成功 metadata 与 6 份撤权投影。真实插件 19 个 wire DTO（18 report + 1 超时 receipt）、真实模型 2 份报告另以冻结 Schema 检查。Schema 脚本中的 41 个语义用例仍只校验声明完整性，不把它计作实际执行了 41 场景；实际语义以上表 Go 断言为准。系统失败的真实 Trace 失败节点、实时 code/stage 和持久化 Worker failed task/event 由 runtime/agent_run 的现有生产链回归检查。

## 本地检查结果

Skills 集成包全量（含原有夹具）通过；evidence/skills/runtime/agent_run、Agent 授权、bootstrap/seed 相关单测通过，四个核心包 race、相关 vet、Core 构建及 diff 检查通过。真实模型、native 与前端原件各命令的日志/摘要原件随本次交付保留；不复制私有运行配置。

首次检查时 `upgrade_registry.py --check` 报告 `UPGRADE_PAYLOAD_CHANGED`，差异组为 core/catalog。后续继续验收时已核对集中迁移新增的 EvidenceSnapshot 和 IAM 连续授权基线模型，以及新的正式 IAM Capability，再统一生成内容寻址清单。当前清单检查、`make capability-check`、database/seed/database persistence 测试及 database/Core 两个入口构建通过；旧、新载荷摘要保留在本轮 `upgrade-payload-review.json`，没有覆盖旧数据库升级履历。

## 当前环境的剩余验收

用户已明确授权停止旧 Core PID 14721/15233；实际服务收到 SIGTERM 后退出，父级 `go run` 随之退出，8077 已释放。停机后另存 custom-format 备份（40,201,651 字节，9265 个目录项）。随后实际执行集中迁移成功：本机 `powerx.public.agent_evidence_snapshots` 已存在，`payload` 类型为 `bytea`；用户 14、成员 15、角色 22、租户 4、Agent 17、Draft 4 和旧 Revision 53 的数量与迁移前一致，既有四份示例 Revision 的内容摘要也一致。

首次执行 `database migrate -config etc/config.yaml` 时，`config.Load` 在数据库连接之前拒绝当前公开的旧默认 JWT 密钥，该次未写库。后续将数据库操作与持久登录配置修复分开：迁移和示例发布进程通过正式 `POWERX_AUTH_JWT_SECRET` 环境覆盖读取有效的随机运维配置；这些命令不签发 JWT，不修改持久签名配置。迁移成功后逐字节核对 `backend/.env` 和 `backend/etc/config.yaml` 均保持原样。候选持久换钥方案没有应用，现有默认密钥问题仍是独立配置事项；未降低密钥校验。

`seed-native-marketing-skills` 实际发布成功，仅汇总示例新增 V5 Revision `b92c4d1a-ae23-4bc3-a80a-00b5dcbc6d9a`，总 Revision 数从 53 增至 54；其余三个当前 Revision 保持原指针。新建 Service/SQL 回读确认发布状态和 `powerx.agent.response/v5`。再次执行相同发布命令，Revision 数量和指针不变，四个旧 Revision 原件仍保留。

同时发现同一工作区后续改动使新 Core 构建失败：`host_config.go` 已将 config 包导入为 `coreconfig`，新增调用仍写成 `config.BackgroundWorkersDisabled`。已修正为现有别名；当前 Core 重新构建、manager/router 回归通过。

迁移前已完成本机 custom-format 备份约 40 MB，`pg_restore --list` 成功读取 9265 个目录项；保存核心对象数量、现有营销 Revision UUID/内容摘要，并在迁移和发布后核对。备份及凭证只保存在私有临时证据目录。独立 Playwright Chromium 143 使用服务器签发的真实凭证，Core 登录、菜单和用户上下文均返回 200，没有注入 test-token、跳过鉴权或 mock API。此前 CUA 无浏览器、Chrome MCP profile 被占用的问题由独立 Chromium 验收解决。

新 Core 与独立 Chromium 使用私有验收配置和真实登录运行；不修改日常配置，不启动或重启用户插件。临时服务记录 PID、命令、工作目录和端口，验收结束后关闭并检查端口释放。

真实绑定 Skill 的聊天发现原文传递遗漏：Worker 恢复了持久化 `input.Message`，但只为 workflow/llm 补入参数；Skill 收到 Planner 摘要，缺少合同要求的 `/message`，实际任务报 `evidence.source_missing`，页面正确显示失败，没有成功报告。已在通用执行入口保存可信原文，在 Manager 最终 payload 处恢复；同步、持久化和嵌套任务共用，结构化工具数值输入保持独立。新增精确原文字节、Planner 改写、嵌套 payload、ParamRefs、不修改计划、结构化字段保留和真实 Worker 调度回归；agent/runtime/skills/bootstrap 单测、agent/runtime race/vet 与 Core 构建通过。

随后实际任务完成、PostgreSQL 保存了两份工具回执，聊天终态却迟迟没有写入。原因是任务结果摘要在回读时经 `interface{}` 解码再编码，typed labels/reference 等结构体变成 map 后改变字段顺序，被误判为原件损坏。现在先验证保存的原始结果字节再解码，通用数值使用 `json.Number`；保留 locator/身份/摘要/成功状态的检查。新增结构体、大整数小数、篡改拒绝及 Worker → 持久化 → typed labels/报告回读测试。新 Core 从已有对象存储结果恢复出助手消息 374，不重跑模型或工具；归档 32619 字节、摘要验证通过，归档 `/message` 与实际用户消息逐字相同，两个计算结果为 1.35/0.85。

恢复出的报告已在真实页面显示、刷新后重新选择历史会话回显，Trace HTTP 200、completed、零任务错误。另一个新会话 109 在不中断服务的情况下实时生成 V5，计算仍为 1.35/0.85。工作台刷新后默认欢迎页，须在侧栏重新选择会话；没有把自动恢复选中会话计为通过。更换数值的一次真实 Planner 输出为 Markdown 及参数 JSON，却因缺 `tool_calls` 被旧解析器当成合法空选择，最终生成普通对话占位消息；该次未计为 V5 验收。已补显式数组校验，格式错误返回 `agent.planner_decision_invalid`，合法空数组保留原有流程；新构建上的真实截断 JSON 已验证进入 failed，Trace 保留失败事件，不再生成完成占位消息。

后续定位截断原因是 Planner 复制整篇原文到未声明参数中。通用提示现明确要求没有声明参数时使用 `{}`，reason 不复述原文；并设置 `TemperatureSet=true`，使 Planner 原本声明的零温度实际传给支持该标记的模型驱动。没有增加业务 Skill ID 分派、放宽 Schema 或改写模型响应。

包含全部修复的新构建再次走完整页面流程，成功会话 112 / 助手消息 384 / run `3a77edd5-9e85-40cd-95a7-53705ff33929`：

- 替换材料为投入 20.0、GMV 60.0、增量 GMV 10.0；真实工具计算 3/0.5，页面按 locale 展示 3.00/0.50。
- 保留原文 1.35/3.37，与计算值形成两条独立冲突；27.8% 保持原文百分比，复购人数缺口仍在。
- 实时展示、刷新后选择同一会话回显、点击“追踪本轮”均通过；Trace HTTP 200、completed、零任务错误，页面没有 JavaScript 异常。
- PostgreSQL 归档 32368 字节，两份实际工具回执，摘要 `sha256:e3512a727f97f3bbd63251988dcf02cf2df7041f78c57c1f7cdc3cb4529674b2`；归档 `/message` 与实际聊天原文逐字相同。

验收已通过正式 API 删除 8 个测试会话和唯一测试 Agent，并注销当前测试登录的 refresh token。软删除记录及验收原件保留用于审计；原用户/成员/角色/租户/Agent/Draft 数量保持原值，Revision 为预期的 54，四个旧 Revision 内容摘要一致，V5 发布指针保持不变。临时 Core PID 10988、浏览器 launcher 35491、Chromium 35557 及子进程全部退出，8077/9001/49616 监听释放；用户前端、模型和插件服务未停止。

本机交付与部署边界：

1. 集中迁移和内置示例 V5 发布均已完成，并已核对真实 PostgreSQL 存储类型、旧数据保留和发布幂等性。
2. 同源新 Core 临时进程及独立 Chromium 已完成聊天提交 → V5 展示 → Trace → 页面刷新 → 历史回读。调试使用临时有效签名配置和真实登录，结束后关闭；默认 E2E auth 文件仍使用 test-token，本次不使用该文件。
3. 实际用户插件的 native 安装和授权状态仍需逐安装核对；前面的 native 工具矩阵使用真实隔离安装和进程，不能冒充所有当前插件已升级。
4. 日常服务仍由用户启动。当前本机持久 JWT 配置使用被新校验拒绝的公开默认值，正式日常启动前须另行配置有效密钥；本次没有应用持久换钥。现有登录会话可能受换钥影响，应按单独的配置变更处理。本机 `go run` dev 挂载不能充当固定 native 实现证明。

## 远程服务器部署本次变更

结构迁移针对数据库执行。本机完成不会更新远程数据库；每个独立远程数据库部署包含这次结构变更的新版本时，也需要显式迁移。若发布流水线已有迁移步骤，就由该步骤执行；普通服务启动和 release 切换不自动执行 migrate/seed。

systemd 部署示例：先备份目标实例、停止 Core/Worker 写入，使用同一新 release 的 `database` 和 Core 二进制，在对应实例 backend 目录执行：

```sh
sudo ./database migrate -config /etc/powerx/config.yaml
```

开发实例使用 `/etc/powerx-dev/config.yaml`。命令读取该配置同目录的 `powerx.env`，沿用目标实例自己的有效密钥、数据库和存储配置；本机临时运维密钥不复制到服务器。若还要升级上述内置汇总示例，再执行专用发布命令：

```sh
sudo ./database seed-native-marketing-skills -config /etc/powerx/config.yaml
```

这一步保留历史 Revision，仅发布有差异的内置示例；不要为此执行完整 seed 或 refresh。共享同一数据库的多个 Core 实例统一安排一次迁移，完成后启动同一构建的 Core/Worker，并复核 V5 实际计算、Trace 和刷新历史。再次运行相同示例发布已在本机实际验证幂等。

Docker 发布使用仓库已有的 `migrate --action=status|plan|apply` 及目标 release/source/plan 参数流程，不能照搬 systemd 路径；对应流程见 [容器升级说明](../../deploy/powerx/container/README.md)。完整 systemd 配置及发布方式见 [服务器部署说明](../../docs/guides/deploy/025-powerx-docker-systemd/systemd/01-deploy-config-start.md)。

T205 的本机分层验收已完成；不把远程发布或其他插件的独立升级计为已执行。所有测试拥有的原生进程均在 cleanup 中停止并检查端口释放，暂停进程先恢复再停止。旧 Core 已按上述授权停止；用户的模型服务和插件进程未关闭或重启。
