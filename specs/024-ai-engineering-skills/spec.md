# Feature Specification: PowerX Skills 管理与治理

**Feature Branch**: `024-ai-engineering-skills`  
**Created**: 2026-03-09  
**Status**: Draft  
**Input**: User description: "请根据 docs/plan/ai_engineering/skills 的开发计划文档，生成对应的 spec 文档"

**Developer Guide**: 插件侧 Agent/Skill 注册和 Skill Bridge 落地总入口见 [PowerXPlugin Agent / Skill Bridge 开发指南](../../docs/guides/develop/plugin_agent_skill_bridge.md)。

## Clarifications

### Session 2026-03-09

- Q: 第三方 Skill 的首版导入入口范围是什么？ → A: 支持上传 Bundle + 填写来源元数据（URL/Tag）用于审计，不自动拉取。
- Q: 当调用请求未显式指定 version 时，默认使用哪一个版本？ → A: 默认使用该 skill_id 的最新已发布版本。
- Q: Skill 的发布审批策略首版采用哪种？ → A: 所有 Skill 一律人工审批后发布。
- Q: PowerX 官方固有 Skills 首版的数据来源定义是什么？ → A: 后端内置官方目录表，随平台版本发布。
- Q: 发布前的完整性校验门槛首版如何设定？ → A: checksum 必须，signature 可选（按环境策略）。

### Session 2026-03-19

- Q: Skill 路由是否仅支持单技能命中？ → A: 否，目标态必须支持多技能候选同时识别，并由 Planner 进行串行/并行编排。
- Q: Skill 调用是否以 tenant 直调为主入口？ → A: 否，Agent invoke 应作为主闭环入口，tenant 直调与 unified invoke 作为底层执行接口保留。
- Q: 语义匹配阶段是否只做规则/词法重排？ → A: 否，目标态需要引入 LLM Tool-Calling 决策环，输出结构化执行计划（含依赖关系与并发组）。

### Session 2026-06-07

- Q: PowerX 与插件之间的 Agent Skill 机制如何定位？ → A: 定义为 PowerX Agent Skill Bridge，由 PowerX 统一承接渠道、会话、Agent Runtime、权限、租户与 Skill 治理，插件声明 Skill 源定义与 action-capability 映射，业务执行统一走 Capability Invocation。
- Q: 插件是否可以有自己的 Skill 定义？ → A: 可以。插件侧 Skill 是源定义态能力包；PowerX 侧 Skill 是治理态平台能力。插件 Skill 必须经 PowerX 导入、校验、审批发布后才能进入 Agent 候选池。
- Q: 插件自有 Chat 是否可以直接调用插件业务接口？ → A: 不可以作为长期路径。插件自有 Chat 必须通过 PowerXPlugin Framework Client 调用 PowerX Agent Session/Stream API，再由 Agent Runtime 将 Skill action 映射为 capability 调用。

### Session 2026-06-08

- Q: Agent 多轮会话与单轮消息如何给开发者调试？ → A: 建立 PowerX Agent Run Trace & Report，按 Session/Message/Node 三层记录结构化运行时日志，本地写入 `backend/logs/agents`，生产写入 Loki，并提供 root-only 页面和报告下载。
- Q: 这套日志是否复用普通 backend logger？ → A: 不直接复用。必须封装独立 `AgentTraceLogger`，普通 logger 只负责服务日志，AgentTraceLogger 负责可回放运行轨迹。
- Q: 谁可以查看和下载智能对话报告？ → A: 仅 root 用户，后端接口必须强制校验 root 权限，非 root 返回 `AGENT_TRACE_ROOT_REQUIRED`。
- Q: PowerXPlugin 是否需要维护插件 Agent/Skill 记录？ → A: 需要。插件侧维护插件 Registry/Sync 记录作为开发态与声明源，PowerX 底座维护治理态与运行态记录作为 Agent Runtime 权威源。
- Q: 插件侧创建 Agent/Skill 后是否只保存在插件自有？ → A: 不可以。插件 backend 必须通过 PowerX Admin/Skill/Agent API 同步生成底座记录，并回写 `powerx_agent_uuid`、`powerx_skill_id`、同步状态与错误信息。
- Q: 插件前端是否可以直接调用 PowerX Admin API 创建 Agent/Skill？ → A: 不可以。调用链路必须是 `PowerXPlugin Web -> Plugin Backend Proxy -> PowerX Admin/Agent/Skill API`。

### Session 2026-06-09

- Q: PowerX Skill 的源格式是否可以只是数据库 metadata 或 Go struct？ → A: 不可以。PowerX 统一采用 `SKILL.md` 目录包作为 Skill Package 源格式，数据库保存解析后的治理态与运行态索引。
- Q: 插件 Skill 包与 PowerX 入库 Skill 是否冲突？ → A: 不冲突。插件交付 `SKILL.md` 包，PowerX 解析、校验、计算 checksum 后入库；Agent Runtime 调用数据库治理态记录，并通过 source/checksum 追溯到原始 Skill Package。

### Session 2026-06-24

- Q: 多任务、多智能体、缺参等待和执行结果应该用什么协议在 PowerX 与 PowerXPlugin 页面统一展示？ → A: 定义 PowerX Agent Run State Protocol。它不是 Google A2A 本身，而是 PowerX Runtime、Trace、Web Admin 与插件调试页共享的 `agent_run.*` 状态协议。
- Q: Core 是否可以为某个插件业务硬编码缺参字段、结果链接或成功文案？ → A: 不可以。Core 只实现通用状态机、参数校验、trace 和 UI 协议；业务字段、slot 映射、结果展示来自 Agent persona/prompt_seed 与 Skill manifest。
- Q: 实时 SSE 能否作为历史权威？ → A: 不可以。SSE/WS 只负责实时事件；历史恢复以 PostgreSQL 的 session/message/message meta/run state snapshot 和 Agent Trace artifact 为权威。

### Session 2026-08-29

- Q: Skill 或租户自建 Agent 是否各自决定最终回复的文本格式？ → A: 不可以。Skill、Tool 与子 Agent 只返回结构化业务事实；PowerX Core 定义统一的 `powerx.agent.response/v1` 最终答复契约，Web Admin 按当前 locale 统一渲染 Markdown Preview。
- Q: 用户在团队会话中追问一个风险点时，是否可以重放上一次固定报告？ → A: 不可以。最终答复必须直接回答当前消息；若需要执行检查，计划必须针对当前问题生成。无真实证据时只能返回 `needs_action` 或 `blocked`，不得声称完成。
- Q: 发布准备演示的固定示例文本能否作为发布准入结论？ → A: 不可以。它只验证运行链路；真实发布准入必须带可核验的验收项和证据。

### Session 2026-08-30

- Q: 固有智能体和固有团队是否可以在 Core 中按业务 Skill ID、团队 Key 或显示名实现执行逻辑？ → A: 不可以。固有对象只是平台随附的 Package/seed 配置；固有与客户自建对象都必须由同一份已发布 Skill Manifest 和团队编排图驱动。
- Q: 文本分析类自建 Skill 如何复用平台模型调用而不改 Core？ → A: Skill Package 声明 `executor.type=llm_prompt`、本地化 Prompt、输入/输出 Schema 和模型策略；Core 的通用 dispatcher 调用唯一 AI Service/LLM Service，不识别业务 Skill ID。
- Q: 普通用户是否必须通过编写或上传 `SKILL.md` 才能创建 Skill？ → A: 不必须。数据库版本快照是创作和运行时权威；Package 仅是从已发布定义生成的导出物，或作为导入草稿来源，二者最终进入相同的审核与发布流程。
- Q: 普通用户创建或修改 Skill 的主要入口是什么？ → A: 用户可在对话中委托具备 `skill_definition` 权限的 PowerX 主智能体生成版本化 Draft；主智能体只能调用受审计的创作服务，不能直接写数据库或自动修改已发布版本。
- Q: 如何兼容 Claude Code/Codex 等外部 Skill？ → A: 导入其共同的 `SKILL.md` + YAML frontmatter 作为标准核心；PowerX 专属的 Schema、权限、执行器和模型策略置于可选 `powerx/` 扩展目录。只有标准核心的包导入为 `instruction_only` Draft，未补全 PowerX 合同前不可执行。

## User Scenarios & Testing *(mandatory)*

### User Story 1 - 管理员统一管理 Skills 生命周期 (Priority: P1)

作为平台管理员，我希望在一个统一入口完成 Skill 的登记、发布、回滚和停用，以便可控地管理平台能力并降低运营风险。

**Why this priority**: 这是 Skills 能力落地的核心价值，没有生命周期管理就无法安全上线和稳定运营。

**Independent Test**: 仅实现管理端生命周期流程即可验证；管理员可完成单个 Skill 从草稿到发布再回滚的闭环操作。

**Acceptance Scenarios**:

1. **Given** 存在一个已登记的草稿 Skill，**When** 管理员执行发布，**Then** 该 Skill 进入可调用状态并记录操作审计。
2. **Given** 存在一个已发布版本和一个历史稳定版本，**When** 管理员执行回滚，**Then** 调用流量切换到历史稳定版本且历史记录保留。
3. **Given** 管理员筛选指定状态和来源，**When** 查看列表，**Then** 系统返回符合条件的 Skills 及其当前状态。

---

### User Story 2 - 开发者与第三方可受控导入 Skills (Priority: P1)

作为插件开发者或生态合作方，我希望将 Skill 按规范导入平台并通过校验后进入草稿状态，以便在不破坏安全边界的前提下扩展能力。

**Why this priority**: 生态扩展是 Skills 的关键目标，受控导入是开放能力与安全治理的平衡点。

**Independent Test**: 仅实现导入与校验流程即可验证；用户提交完整元数据后可导入成功，提交不完整或不可信内容时被拒绝。

**Acceptance Scenarios**:

1. **Given** 导入内容包含完整来源与完整性信息，**When** 发起导入，**Then** 系统创建草稿版本并保留可追溯来源信息。
2. **Given** 导入内容缺少必要完整性信息，**When** 发起导入，**Then** 系统拒绝导入并返回明确原因。
3. **Given** 同一 Skill 已存在已发布版本，**When** 导入新版本，**Then** 系统保留多版本并禁止覆盖已发布内容。

---

### User Story 3 - 租户与 Agent 通过双路径调用 Skills (Priority: P2)

作为租户应用或 Agent 使用方，我希望通过独立调用入口或统一调用入口使用 Skill，并获得一致的结果与错误语义。

**Why this priority**: 双路径一致性决定了 Skills 能否真正成为平台一等能力，并与现有能力体系协同。

**Independent Test**: 仅实现一次双路径调用比对即可验证；同一 Skill 在两条路径下返回一致状态、追踪信息和错误分类。

**Acceptance Scenarios**:

1. **Given** 一个已发布且可见的 Skill，**When** 通过独立调用入口执行，**Then** 返回标准化执行结果与追踪标识。
2. **Given** 同一个 Skill 已绑定统一能力入口，**When** 通过统一入口调用，**Then** 返回与独立入口一致的业务结果语义。
3. **Given** 调用方缺少权限，**When** 发起调用，**Then** 系统拒绝执行并返回可识别的授权错误。

---

### User Story 4 - 审计与隔离满足治理要求 (Priority: P2)

作为平台安全与运维负责人，我希望每次 Skill 调用和关键管理动作都有可追溯审计，并确保租户隔离，以便快速排障和满足合规要求。

**Why this priority**: 安全、审计和隔离是平台级能力的上线前提。

**Independent Test**: 仅实现审计记录与租户隔离校验即可验证；跨租户访问被阻断，且关键操作可按追踪标识回放。

**Acceptance Scenarios**:

1. **Given** 任意一次 Skill 调用完成，**When** 安全审计查询该记录，**Then** 可看到租户、Skill、版本、入口和状态等关键字段。
2. **Given** 用户尝试访问其他租户的执行记录，**When** 发起查询，**Then** 系统拒绝并不泄露目标租户信息。

---

### User Story 5 - 主 Agent 发起 A2A 多 Agent 协作 (Priority: P2)

作为平台 AI 应用负责人，我希望主 Agent 能把复杂任务拆分给多个子 Agent 并汇总结果，以便在保持租户隔离与审计可追溯的前提下提升任务完成质量与稳定性。

**Why this priority**: 现有单 Agent 编排已可用，但复杂任务在“规划、检索、执行、复核”多环节下需要明确的 A2A 分工与调度协议。

**Independent Test**: 通过 PowerX Core seed 初始化一个营销活动复盘团队（1 个主 Agent + 3 个子 Agent + 4 个内置营销 Skills），然后显式执行包含 3 个 `agent_handoff` 节点和 1 个汇总节点的计划；该测试不得依赖 PowerXPlugin、MediaX、AI Craft 或插件 capability handler。

**Acceptance Scenarios**:

1. **Given** seed 已创建营销负责人、内容营销、活动复盘分析、专家知识策展四个 Agent 和“营销活动复盘协作团队”，**When** 重复执行 seed，**Then** Agent、Skill、Binding、Team 与 TeamMember 均按 upsert 语义保持幂等。
2. **Given** 主 Agent 收到活动复盘材料，**When** 系统执行营销活动复盘计划，**Then** 系统按依赖执行素材解析、活动复盘、知识草稿三个子 Agent handoff，并由主 Agent 汇总可审核的方法论草稿。
3. **Given** 营销活动复盘演示中的任一子 Agent 执行失败，**When** 该团队采用固定 `fail-fast` 策略，**Then** 系统立即停止下游节点，整轮运行标记为失败，并返回失败节点和恢复动作；不得生成部分成功报告。
4. **Given** 子 Agent 访问上下文，**When** 请求超出授权上下文范围，**Then** 系统阻断访问并记录拒绝审计。

---

### User Story 6 - 插件通过 Agent Skill Bridge 暴露领域能力 (Priority: P1)

作为插件开发者，我希望通过 PowerXPlugin Framework 声明 Skill metadata、prompt/schema 和 action 到 capability 的映射，并由 PowerX Agent Runtime 统一规划，再通过 PowerX Capability Invocation 执行业务，以便我的插件能力可以被 Telegram、SCRM、移动端和 Web Chat 等渠道复用，而不需要每个渠道直接适配插件业务 API。

**Why this priority**: 插件生态需要统一的 Agent 能力暴露机制；如果渠道直连插件业务接口，会破坏会话、权限、租户和审计边界。

**Independent Test**: 安装一个示例插件，PowerX 发现其 `GET /api/v1/plugin/skills` 输出的 Skill，审批发布并绑定 Agent；用户通过 Agent Stream 发起自然语言请求后，PowerX 从 Skill Manifest 解析 `action_capabilities` 得到 `capability_id`，再调用 Capability Invocation，插件 capability handler 收到完整租户/用户/会话上下文并返回结构化结果。

**Acceptance Scenarios**:

1. **Given** 插件暴露合法 Skill Manifest，**When** PowerX 执行插件 Skill 发现，**Then** 系统导入为 `source=plugin` 的治理态 Skill，并保留 provider、executor、schema 与 checksum 快照。
2. **Given** 用户从任意渠道进入 PowerX Agent Session，**When** Agent Runtime 命中插件 Skill，**Then** PowerX 通过 Agent Skill Bridge 将 action 映射到 capability，并携带 `tenant_uuid/user_uuid/agent_id/session_id/message_id/trace_id` 调用 Capability Invocation。
3. **Given** 插件自有 Chat 页面发送消息，**When** 页面通过 Framework Client 调用 PowerX Agent Stream，**Then** 执行链路与生产渠道一致，不直接调用插件领域业务接口。
4. **Given** capability 调用缺少租户、用户、会话或 trace 上下文，**When** 插件 capability handler 收到调用，**Then** 插件必须 fail-fast，并返回稳定错误码。

---

### User Story 7 - Root 开发者查看 Agent 执行日志并下载报告 (Priority: P1)

作为 PowerX root 开发者，我希望按租户、Session、Message 和 Node 查看 Agent Runtime 的结构化执行轨迹，并下载智能对话报告，以便清楚知道一轮对话中 Agent 接收了什么、识别了什么、调用了什么、为什么失败或如何生成最终回复。

**Why this priority**: Agent Skill、A2A、插件 Skill Bridge 都依赖可观测链路排障；没有结构化 Agent Trace，开发者只能从分散日志中猜测执行路径，无法稳定复盘。

**Independent Test**: 触发一次 Agent Stream 请求后，root 用户可在本地 `backend/logs/agents/{tenant_uuid}/{session_id}/{message_id}` 看到 `run.json/timeline.jsonl/nodes/*.json`，并通过 API 下载 Message 报告；非 root 用户访问同一 API 被拒绝。

**Acceptance Scenarios**:

1. **Given** 用户发送一条 Agent 消息，**When** Agent Runtime 执行 `receive_message -> context_load -> intent_recognition -> planner -> skill_invoke -> final_response`，**Then** 系统为每个节点记录 start/end 或 error 事件，并生成可回放 timeline。
2. **Given** PowerX 运行在本地开发模式，**When** Agent Run 完成，**Then** 系统将结构化日志写入 `backend/logs/agents/{tenant_uuid}/{session_id}/{message_id}/`。
3. **Given** PowerX 运行在生产模式且启用 Loki Sink，**When** Agent Run 执行，**Then** 系统将同一事件模型写入 Loki，并可按 `tenant_uuid/session_id/message_id/run_id/node_kind/status` 查询。
4. **Given** root 用户查看某个 Message Trace，**When** 点击下载报告，**Then** 系统返回 `report.md` 或 `report.json`，内容包含 Summary、User Message、Runtime Timeline、Skill/Tool Invocation、Final Response、Errors/Warnings。
5. **Given** 非 root 用户访问 Agent Trace 查询或下载接口，**When** 请求到达后端，**Then** 系统返回 `AGENT_TRACE_ROOT_REQUIRED`，且不泄露目标 trace 内容。

---

### User Story 8 - 插件自有 Agent/Skill Plugin Registry 同步到底座运行态记录 (Priority: P1)

作为插件开发者，我希望 PowerXPlugin 在本地维护 Agent 与 Skill 的开发态记录，同时通过后端代理同步到底座形成 PowerX 治理态 Skill、运行态 Agent 与绑定关系，以便插件可以对齐 PowerX 底座管理体验，并保证本地调试使用的仍是 PowerX Agent Runtime。

**Why this priority**: 插件只做临时 manifest 暴露无法支撑长期开发调试、同步状态排障和 Agent/Skill 对称管理；但如果插件绕过底座自建 Agent，则会破坏 PowerX 会话、权限、租户和审计边界。

**Independent Test**: 在 PowerXPlugin 创建一个模板对象 CRUD Skill 与一个绑定该 Skill 的 Agent；插件自有保存 Local 记录后，通过插件 backend 调用 PowerX API 创建或更新底座 Skill/Agent；调试页选择该 Agent 创建 PowerX Session 并通过 SSE 触发插件 capability handler。

**Acceptance Scenarios**:

1. **Given** 插件自有创建合法 Skill Local，**When** 用户点击同步到底座，**Then** PowerX 创建或更新 `source=plugin` 的治理态 Skill，并返回 `powerx_skill_id/status` 给插件保存。
2. **Given** 插件自有创建 Agent Local 并选择已同步 Skill，**When** 用户点击同步到底座，**Then** PowerX 创建或更新 Agent，并写入 Agent-Skill Binding。
3. **Given** 插件自有 Agent 未同步或同步失败，**When** 用户进入 Agent Chat 调试，**Then** 页面不得把该 Agent 作为可运行 Agent 使用，并显示同步错误。
4. **Given** 插件前端发起 Agent/Skill 管理动作，**When** 网络请求发生，**Then** 请求必须先到插件 backend proxy，再由插件 backend 调用 PowerX 底座接口。
5. **Given** PowerX 底座侧 Agent/Skill 被停用、删除或发布状态变化，**When** 插件执行重新同步或状态刷新，**Then** 插件自有 Local 必须更新 `sync_status/sync_error/last_sync_at`，并禁止继续调试不可运行对象。

---

### Edge Cases

- 当同一 `skill_id` 同时存在多个可用版本时，如何明确“当前生效版本”并避免歧义调用？
- 当 Skill 已绑定能力后被停用或下线时，系统如何避免出现悬空调用入口？
- 当执行处于安全模式且 Skill 被判定为高风险时，系统如何阻断执行并给出可操作反馈？
- 当导入来源发生变更但版本号未变时，系统如何防止隐式覆盖和来源漂移？
- 当调用链路中出现临时失败时，系统如何区分可重试与不可重试错误，避免错误重试放大影响？
- 当渠道插件或本地调试页试图直接调用业务插件私有 API 时，系统如何阻止绕过 PowerX Agent Runtime 的非标准路径？
- 当插件声明的 executor capability 与 PowerX Registry 中的 capability 绑定不一致时，系统如何拒绝调用并保留审计？
- 当 Agent Trace artifact 包含 prompt、上下文或 executor payload 时，系统如何脱敏并避免下载报告泄露敏感数据？
- 当本地文件 sink 与 Loki sink 同时启用时，系统如何保证事件模型一致且报告生成优先级明确？
- 当插件自有 Local 与 PowerX 底座记录发生漂移时，系统如何明确以 PowerX 运行态为准，并要求插件重新同步？
- 当插件自有 Agent 绑定了未发布或未同步的 Skill 时，系统如何拒绝同步并给出可操作错误？

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: 系统必须支持将 Skill 作为独立能力对象进行登记、查询和状态管理。
- **FR-001a**: 系统必须支持展示 PowerX 官方固有 Skills 目录，且首版目录来源为平台内置官方目录表。
- **FR-002**: 系统必须支持 Skill 的生命周期状态流转，至少包含草稿、发布、弃用与停用。
- **FR-003**: 系统必须支持同一 `skill_id` 的多版本并存，并确保已发布版本内容不可被覆盖。
- **FR-004**: 系统必须保证每个 Skill 在任一时刻只有一个当前生效的发布版本。
- **FR-005**: 系统必须支持将 Skill 与统一能力入口进行绑定，使其可通过统一调用路径被路由。
- **FR-006**: 系统必须支持开发者与第三方在受控流程中导入 Skill，并在发布前进入草稿状态。
- **FR-006a**: 首版第三方导入必须通过上传 Bundle 完成，并允许附带来源元数据用于审计追溯。
- **FR-006b**: 首版不支持从远程代码仓在线拉取并直接导入 Skill。
- **FR-007**: 系统必须对导入内容执行规范校验与完整性校验，不满足条件的导入必须被拒绝。
- **FR-007a**: 系统必须支持 `SKILL.md` 目录包作为标准 Skill Package 源格式，至少解析 YAML frontmatter、Markdown body、schema 引用、executor 声明与包 checksum。
- **FR-007b**: 系统必须在 Skill Registry 中保存 `source_format/package_uri/raw_markdown/frontmatter_json/body_markdown/package_checksum`，以支持审计、导出和漂移检测。
- **FR-007c**: 系统不得将仅存在于 Go struct 或临时 HTTP DTO 的 Skill 定义作为长期源格式；此类定义必须迁移或导出为 `SKILL.md` 包后才能进入标准发布流程。
- **FR-007a**: 首版发布前必须校验 `checksum`，缺失或不匹配时必须拒绝发布。
- **FR-007b**: 首版 `signature` 默认可选，但系统必须支持按环境策略启用为强制校验。
- **FR-008**: 系统必须在调用前执行权限检查，至少覆盖租户身份、能力可见性与授权策略。
- **FR-009**: 系统必须同时支持独立 Skill 调用与统一调用入口，且两条路径在结果语义上保持一致。
- **FR-009a**: 当调用请求未显式指定版本时，系统必须自动路由到该 `skill_id` 的最新已发布版本执行。
- **FR-010**: 系统必须为每次调用返回统一结果模型，包含追踪标识、执行状态和结果内容。
- **FR-011**: 系统必须记录关键管理动作与调用审计，保证来源、操作者、版本与结果可追溯。
- **FR-012**: 系统必须执行多租户隔离，禁止跨租户访问 Skill 执行数据与审计记录。
- **FR-013**: 系统必须支持发布回滚并保留完整历史，回滚不应删除历史版本。
- **FR-013a**: 首版所有 Skill 发布都必须经过人工审批，未经审批的草稿版本不得对租户开放调用。
- **FR-014**: 系统必须支持插件卸载前的绑定关系处置，避免遗留不可用调用入口。
- **FR-015**: 系统必须支持在安全模式下对高风险 Skill 进行阻断或限制执行。
- **FR-016**: 系统必须对外提供明确且稳定的错误分类，至少覆盖未找到、版本不存在、权限拒绝、执行失败与来源不可信。
- **FR-017**: 当 Agent 进行 Skill 候选匹配时，系统必须先执行基于结构化约束的硬过滤（至少覆盖租户可见性、发布状态、权限约束与来源策略），再进入语义候选阶段。
- **FR-018**: 系统必须支持“硬过滤 + 候选召回 + 重排决策”的多阶段匹配流程，并保证最终仅从硬过滤后的候选集合中做选择。
- **FR-019**: 当单 Agent 可用 Skill 规模达到高基数（例如 10k 量级）时，系统必须具备可扩展的候选检索能力，并支持通过缓存/索引机制避免全量扫描。
- **FR-020**: 系统必须支持租户管理员在可视化管理界面配置 Skill 来源策略（source allowlist），并保证该策略可作用于统一入口 `preferred_protocol=skill` 的候选过滤流程。
- **FR-020a**: source allowlist 至少支持 `builtin`、`plugin`、`third_party` 三类来源，且配置变更需可持久化与可追踪。
- **FR-021**: 系统必须支持在单次 Agent 调用中识别多个 Skill 候选，并返回结构化候选列表（含置信度、理由与约束）。
- **FR-022**: 系统必须支持 Planner 基于多候选构建执行计划，计划节点至少覆盖 `workflow|skill|tooling|llm` 四类，且支持串行依赖、并行分组、失败策略（fail-fast/continue）。
- **FR-023**: 系统必须提供 Agent 主调用入口（invoke/stream）承载“LLM 意图识别 → 计划生成 → workflow/skill/tooling/llm 执行 → 汇总响应”闭环，不要求调用方显式传入 `skill_id` 或 `flow_id`。
- **FR-023a**: Agent 主入口必须在自然语言意图识别前执行结构化 Runtime Intent 路由；`agent.bound_capabilities`、`agent.bound_skills` 等控制面查询必须由确定性 handler 执行，禁止通过自然语言关键词穷举触发。
- **FR-023b**: Runtime Intent 命中后不得进入 LLM、Planner 或全局候选池，返回 metadata 必须标注 `llm_bypassed=true`、`planner_bypassed=true` 与数据源。
- **FR-024**: 系统必须支持 LLM Tool-Calling 作为 Planner 决策的一部分，并将可用 `workflow|skill|tooling` 候选作为受控工具清单注入，禁止选择未授权或未发布对象。
- **FR-025**: 系统必须记录计划级审计与追踪信息，至少包含 `plan_id`、节点拓扑、节点输入输出摘要、节点状态与重试轨迹。
- **FR-026**: 系统必须保证 Agent 主入口、tenant 直接调用、tenant unified invoke 三条路径在错误语义与追踪模型上兼容对齐。
- **FR-027**: 系统必须由 LLM 统一执行意图识别，禁止将“仅 Flow 路由命中”作为 Agent 规划主路径。
- **FR-028**: 当意图识别与候选规划均未命中可执行节点时，系统必须回落为普通上下文对话回复（`llm` 直答），并在流式事件中明确标注未触发编排。
- **FR-029**: 系统必须将 tooling 目录与调用治理以数据库持久化为权威源（capability registry），运行时缓存仅作为加速层，不得作为唯一事实来源。
- **FR-030**: 系统必须将 Agent Executor 中 `node.kind=skill|tooling` 接入真实调用链路（Skill InvokeService/AdapterService、Capability InvocationService），禁止返回仅用于演示的占位结果。
- **FR-031**: 系统必须按能力类型（`workflow|skill|tooling`）与来源层级（`system builtin`、`agent custom`）构建候选池，并在规划前完成合并与去重。
- **FR-032**: 系统必须在候选池构建阶段执行统一硬过滤（租户、权限、发布状态、source allowlist、tool_grants、visibility/scope），LLM 不得看到未授权候选。
- **FR-033**: 系统必须在 LLM 决策输入中提供分层能力清单（workflow/skill/tooling 分区，且标注 `source=system|agent`），避免仅以单段文本拼接导致语义漂移。
- **FR-034**: 系统必须支持组合规划（workflow 节点调用 skill/tooling；skill 内部可封装 workflow/tool/script/mcp）并在计划节点中保留可追溯的 `node_kind/node_ref/source_scope` 元信息。
- **FR-035**: 系统必须为 Agent 主入口建立统一 Context 分层拼装机制（固定前缀、能力清单、会话摘要、最近消息、检索片段、当前输入），并保证层级顺序稳定可复现。
- **FR-036**: 系统必须在每次 LLM 请求前执行上下文预算管理（token budget），在超预算时按策略裁剪检索片段与历史窗口，并保留最小上下文保护集。
- **FR-037**: 系统必须将会话摘要从“占位文本拼接”升级为结构化摘要（至少包含 `facts/decisions/open_issues/constraints`），并支持按阈值与周期刷新。
- **FR-038**: 系统必须支持 Provider 无关的 Prompt/Context Cache 策略（`auto|force_off|force_on`），并通过能力探测决定是否启用缓存优化。
- **FR-039**: 系统必须记录并暴露上下文优化观测字段（至少 `prompt_tokens/completion_tokens/cached_tokens/trim_actions/context_layers_size`），用于排障与成本分析。
- **FR-040**: 系统必须保证 Context 优化不改变主路由原则：`/command` 以规则为快捷入口，其他自然语言请求仍由 LLM 意图识别与规划主导。
- **FR-040a**: 系统必须支持节点级模型选择策略，至少覆盖 `runtime_intent/intent_classifier/planner/skill_param_extractor/final_response/reviewer`；首版允许全部继承 Agent 默认模型，但接口、上下文与 trace 必须保留节点选择结果。
- **FR-040b**: Planner LLM 调用必须读取 `planner` 节点模型选择结果；当未配置节点级策略时，必须严格回落到当前 Agent 默认模型。
- **FR-040c**: Agent 主入口必须引入 ResponsePlanner / Context Builder / Final Response 分层；最终自然语言回复不得直接由“全局候选能力摘要 + 最近消息”通用 prompt 生成。
- **FR-040d**: ResponsePlanner 必须输出结构化 `ResponsePlan`，至少包含 `response_mode/response_intents/answer_requirements/should_call_tool/target_capability_ids/use_capability_context/include_examples/include_schema/repeat_full_intro/needs_clarification/missing_fields`；一条用户消息包含多个问题时必须保留多个 `response_intents`，不得被单一 `response_mode` 或 `recent_capability_intro` 覆盖。
- **FR-040e**: 系统必须支持标准 `response_mode`：`capability_intro`、`capability_howto`、`skill_execution`、`clarify_params`、`normal_chat`、`error_explain`。
- **FR-040f**: Context Builder 必须按 `response_mode` 动态注入上下文；能力介绍和能力使用说明只能读取当前 Agent 已绑定、已发布、租户可见且权限通过的能力，不得读取全局候选池作为用户可见事实。
- **FR-040g**: assistant message 必须持久化 response meta，至少包含 `response_mode/capability_ids/response_plan_id/used_context_layers/tool_calls/final_response_model/model_selection`；后续去重和追问必须基于该 meta，不得基于自然语言文本匹配。
- **FR-040h**: Agent Stream 必须输出 `response_plan` debug event，Agent Trace 必须记录 `response_planner/context_builder/final_response/history_persist` 节点，便于 root 用户回放本轮回答为什么这样生成。
- **FR-040i**: Agent 上下文必须采用驱动分层：Runtime Memory 仅保存本轮过程态，PostgreSQL 作为 session/message/message meta/registry/binding/model policy/context_ref 权威源，Redis 仅作短 TTL 缓存，Local File/Loki 仅作 Trace/Report 存储。
- **FR-040j**: Core Runtime 必须支持从 Agent `persona/prompt_seed` 与 Skill `response_guidance` 组合最终回复规范；Core 只允许实现通用安全和编排约束，禁止硬编码业务 Agent 的字段规则、专属话术或行业流程。
- **FR-040k**: Skill `response_guidance` 必须支持 `general/capability_intro/capability_howto/clarify_params/skill_execution/error_explain` 分组，并在候选能力上下文中保留 mode 标签，供 Final Response 按当前 `response_mode` 使用。
- **FR-040l**: 系统必须定义 Agent Run State Protocol，标准事件以 `agent_run.*` 为前缀，至少覆盖 `started/response_plan/intent_detected/plan_created/task_status/task_started/awaiting_params/task_completed/task_failed/final/ended`。
- **FR-040m**: `agent_run.task_status` 必须携带 `run_id/session_id/message_id/trace_id/task_id/status`，并在适用时携带 `agent_key/agent_name/node_kind/skill_id/capability_id/action/missing_fields/result/links/error`。
- **FR-040n**: Agent task 状态必须使用标准枚举 `pending|awaiting_params|running|completed|failed|skipped`；缺参时必须进入 `awaiting_params`，不得直接伪造成功或吞掉错误。
- **FR-040o**: 系统必须支持 `AgentRunState` 历史快照；页面刷新、从 Chat 跳转 Trace、从 Trace 返回 Session 时必须能恢复本轮 message 的任务状态。
- **FR-040p**: Web Admin Agent Chat、Team Task、Agent Trace 与 PowerXPlugin Agent Chat 调试页必须消费同一套 `AgentRunState` 语义，禁止插件调试页自定义一套私有任务状态协议。
- **FR-040q**: Skill manifest 必须支持 Agent Run State 展示元数据，至少包含 `action_required_args/action_optional_args/slot_mapping/pending_task_policy/result_presentation` 的解析与治理态保存能力。
- **FR-040r**: Final Response 在没有真实 `task_completed`、Skill result、Capability result 或 A2A child result 时，不得输出“已创建/已更新/已删除/已完成”等成功性业务结论。
- **FR-040s**: UI 与历史快照必须区分 Run 完成和 Task 完成：`agent_run.final/ended` 或旧 `final/end success=true` 只能表示本轮回复流程结束，不得驱动业务 task 进入 `completed`；只有 `agent_run.task_completed` 或 task snapshot `status=completed` 且包含真实 `result/links` 时，才能展示“任务完成”。
- **FR-040t**: 所有产生用户可见业务结果的 Skill、Tool 和 A2A 汇总节点必须返回 `powerx.agent.response/v1`。该 envelope 至少包含 `schema/kind/outcome/summary/answer/acceptance/evidence/gaps/next_actions`；Skill 不得以拼接最终 Markdown 代替结构化结果。
- **FR-040u**: Core 必须校验最终答复 envelope。缺少 `answer`、执行结果缺少验收项、`outcome=completed` 缺少可验证证据或状态非法时，必须 fail-fast 为 `agent.response_contract_invalid`，写入 Trace 和用户可操作的错误摘要；不得退回原始文本或通用“任务完成”。
- **FR-040v**: Final Response 必须直接回答当前用户消息。对同一 session 的追问，Runtime 必须依据当前 `ResponsePlan`、SkillState 和已有结构化结果选择解释、补参、局部复查或重新规划；禁止因为团队上下文存在而无条件重放固定任务计划或历史报告。
- **FR-040w**: Web Admin、PowerXPlugin Agent Chat 与消息历史恢复必须使用同一 envelope 渲染器，按当前 locale 输出统一的“结论、直接回答、验收项、证据、缺口/阻塞、下一步”Markdown Preview；Skill manifest 只能补充业务字段说明、结果链接和展示素材，不能定义平台答复区块或标签文案。
- **FR-041**: 系统必须支持主 Agent 在单次请求内创建 A2A 执行计划，并将子任务分发给多个子 Agent（至少支持串行与并行两种调度模式）。
- **FR-041a**: 系统必须提供 PowerX Core 内置 A2A seed 数据，用于初始化营销活动复盘多智能体演示团队；seed 至少包含营销负责人、内容营销、活动复盘分析、专家知识策展四个 Agent，四个 `marketing.*` 内置 Skills、Agent-Skill Binding、Agent Team 与 Team Members。发布准备是可选专项团队，不得作为默认演示验收对象。
- **FR-041b**: A2A seed 必须是 upsert 幂等语义，重复执行不得删除用户已有绑定，不得因唯一索引冲突失败。
- **FR-041c**: 发布准备 A2A MVP 测试必须只依赖 PowerX Core 数据库与运行时，不得依赖插件安装、插件 capability handler 或 PowerXPlugin 本地记录。
- **FR-042**: 系统必须支持 A2A 任务级上下文隔离，子 Agent 仅可读取主 Agent 显式下发的上下文切片与引用，不得默认继承完整会话。
- **FR-043**: 系统必须支持 A2A 协作的失败策略（`fail-fast|continue|retry-once`），并在最终响应中返回子任务级执行状态。
- **FR-044**: 系统必须为 A2A 协作写入全链路审计，至少包含 `team_id/task_id/parent_agent_id/child_agent_id/handoff_trace_id` 与状态变更时间线。
- **FR-045**: 系统必须保证 A2A 子 Agent 调用继续遵循租户隔离与授权约束，禁止子 Agent 越权调用未授权的 `workflow|skill|tooling` 候选。
- **FR-046**: 系统必须保证团队任务工作台按用户所选团队动态路由，禁止在页面逻辑中硬编码固定 `team_id` 或固定主 Agent。
- **FR-047**: 系统必须在团队管理与成员管理界面默认展示 Agent 可读标识（名称/Key），`id` 仅作为辅助信息，不得作为主要识别信息。
- **FR-048**: 系统必须通过平台固定 Team Role 枚举约束 A2A 团队角色；`planner` 只用于 TL/主 Agent 语义，子 Agent 成员只能使用 `retriever/executor/reviewer`，枚举必须集中维护，禁止业务层散落字符串判断。
- **FR-049**: 系统必须在会话界面提供可视化协作过程（Intent/Plan/Node 状态），并支持用户基于页面信息判断“是否发生协作、协作是否完成”。
- **FR-050**: 系统必须提供“页面可见字段”和“审计可查字段”的一致口径，当前端未展示 `team_id/child_agent_id/handoff_task_id` 时，需提供可操作的审计查询路径。
- **FR-051**: 系统必须实现 PowerX Agent Skill Bridge，将 PowerX Agent Runtime、Agent 已绑定 Skill 与 PowerX Capability Invocation 连接为标准机制，禁止渠道、移动端、SCRM 或 Skill 私有 executor 作为长期路径直接调用插件业务接口来绕过 Agent Runtime。
- **FR-052**: 系统必须支持插件侧 Skill 源定义导入，源定义至少包含 metadata、description、intent_examples、input_schema、output_schema（可选）、prompt/response 规范（可选）和 `action_capabilities` / `executor.action_map` 声明。
- **FR-053**: 系统必须区分插件源定义态 Skill 与 PowerX 治理态 Skill；插件声明的 Skill 未经 PowerX 校验、审批和发布前，不得进入 Agent 候选池或 tenant 调用入口。
- **FR-054**: PowerXPlugin Framework 必须提供插件 Skill Runtime 封装，至少包含 `PluginSkillManifest`、`PluginSkillRegistry`、`PluginSkillSchema`、`PluginSkillActionCapabilityMap` 与稳定错误模型。
- **FR-055**: PowerXPlugin Framework Client 必须封装插件访问 PowerX Agent Session 的 HTTP/SSE/WS 通讯能力，插件自有 Chat 和调试页面必须复用该 Client。
- **FR-056**: 插件必须统一暴露 Skill 发现接口，至少包含 `GET /api/v1/plugin/skills`、`GET /api/v1/plugin/skills/:skill_id/schema`；插件不得把 PowerX Capability Invocation 作为标准业务执行入口。
- **FR-057**: PowerX 调用插件 capability handler 时必须注入完整调用上下文，至少包含 `tenant_uuid`、`user_uuid`、`agent_id`、`session_id`、`message_id`、`skill_id`、`trace_id`、`channel`。
- **FR-058**: 插件 capability handler 必须校验调用来源、租户上下文、Skill/Agent 启用状态和 capability；任一关键上下文缺失、action 无映射或 capability 不匹配时必须 fail-fast，禁止匿名 fallback 或跨租户降级。
- **FR-059**: Agent Stream 与插件自有 Chat 必须共享同一事件语义，至少覆盖 `intent`、`plan`、`node_start`、`node_end`、`token`、`final`、`end`。
- **FR-060**: 插件卸载、停用或版本切换时，系统必须同步处置插件来源 Skill、Agent 绑定和 capability 绑定，避免 Agent Runtime 继续路由到不可用 executor。
- **FR-061**: 系统必须提供 Agent Runtime 结构化追踪机制，按 `Session Trace`、`Message Trace`、`Node Trace` 三层记录 Agent 执行路径。
- **FR-062**: 系统必须封装独立 `AgentTraceLogger`，并提供 `StartRun/AppendEvent/StartNode/EndNode/FailNode/CompleteRun/BuildReport` 等标准方法；禁止以普通文本 logger 作为 Agent Trace 主数据源。
- **FR-063**: 本地开发模式必须支持将 Agent Trace 写入 `backend/logs/agents/{tenant_uuid}/{session_id}/{message_id}/`，至少生成 `run.json`、`timeline.jsonl` 和 `nodes/*.json`。
- **FR-064**: 生产模式必须支持将 Agent Trace 写入 Loki，且 label 至少包含 `service/component/tenant_uuid/agent_id/session_id/message_id/run_id/node_kind/status`。
- **FR-065**: 每轮 Agent Message Run 必须具备稳定 `run_id`，并与 `trace_id/session_id/message_id/plan_id/node_id` 关联。
- **FR-066**: Agent Runtime 关键节点必须记录 start/end/error 事件，至少覆盖 `receive_message/session_restore/permission_check/context_load/intent_recognition/planner/skill_invoke/tool_invoke/llm_call/final_response/history_persist`。
- **FR-067**: 系统必须提供 root-only Agent Trace 查询接口，支持按 `tenant_uuid/agent_id/session_id/message_id/run_id/trace_id/node_kind/status/from/to/source` 过滤。
- **FR-068**: 系统必须提供 root-only 智能对话报告下载能力，至少支持 Message 级 `report.md` 与 `report.json`，并预留 Session 级和 zip 下载。
- **FR-069**: 非 root 用户访问 Agent Trace 查询或报告下载接口时，系统必须返回稳定错误码 `AGENT_TRACE_ROOT_REQUIRED`。
- **FR-070**: Agent Trace artifact 必须支持脱敏与大小限制策略；prompt、context、tool payload、executor result 不得默认无控制地进入报告。
- **FR-071**: Agent Trace 必须与 SkillExecutionTrace、Capability InvocationTrace、A2A Handoff Trace 通过 `trace_id/run_id/plan_id/node_id/skill_id/capability_id` 建立关联。
- **FR-072**: 系统必须支持插件 Agent/Skill Plugin Registry 同步语义，接收来自插件 backend proxy 的 Skill/Agent 创建、更新、绑定与状态刷新请求，并生成 PowerX 底座治理态或运行态记录。
- **FR-073**: 系统必须区分插件自有 Local 记录与 PowerX 底座权威记录；Agent Runtime、权限、会话、Trace 与 Skill 候选池只能以 PowerX 底座记录为准。
- **FR-074**: 插件同步 Skill 时，PowerX 必须保存 `provider_plugin_id/plugin_skill_id/plugin_version/manifest_snapshot/executor/capability/checksum` 等来源映射字段，用于审计和漂移检测。
- **FR-075**: 插件同步 Agent 时，PowerX 必须支持写入 Agent 元数据中的 `provider_plugin_id/plugin_agent_id/source=plugin_registry`，并允许同步绑定已发布的插件来源 Skill。
- **FR-076**: 当插件请求同步 Agent 绑定未发布、未审批、不可见或未同步的 Skill 时，PowerX 必须 fail-fast，禁止创建可运行的不完整 Agent。
- **FR-077**: 插件前端不得直接调用 PowerX Admin/Agent/Skill API；PowerX 面向插件的同步请求必须经插件 backend proxy 或受信任插件 runtime 发起，并携带 delegated 鉴权与租户上下文。
- **FR-078**: PowerX 必须为插件 Plugin Registry 同步动作写入审计，至少包含 `provider_plugin_id/plugin_agent_id/plugin_skill_id/powerx_agent_uuid/powerx_skill_id/sync_action/sync_status/operator/trace_id`。

### Key Entities *(include if feature involves data)*

- **Skill Registry Record**: Skill 的注册与治理记录，包含标识、版本、来源、状态、清单快照、完整性信息和绑定关系。
- **Skill Execution Trace**: 一次调用的执行追踪记录，包含追踪标识、租户、Skill、版本、入口、状态、时延与错误摘要。
- **Skill Source Descriptor**: Skill 来源描述，包含来源类型、来源地址、来源版本标识与导入操作者信息。
- **Capability Binding**: Skill 与统一能力入口之间的映射关系，包含可见性和授权约束。
- **Skill Lifecycle Event**: 生命周期关键动作事件，包含导入、发布、回滚、停用等动作及其操作者与时间。
- **Agent Team**: 主 Agent 与子 Agent 的协作编组定义，包含团队成员、角色、权限边界与默认调度策略。
- **Agent Handoff Task**: 一次主 Agent 到子 Agent 的任务交接记录，包含输入摘要、上下文引用、失败策略、执行状态与回传结果摘要。
- **Plugin Skill Definition**: 插件侧源定义态 Skill，包含 metadata、prompt/schema、executor、脚本资源与 provider 信息。
- **Plugin Skill Invocation Context**: PowerX 调用插件 capability handler 时注入的上下文，包含租户、用户、Agent、会话、消息、渠道和 trace 字段。
- **Plugin Skill Result**: 插件 capability handler 返回的结构化结果，包含执行状态、消息、任务标识、业务数据和 trace。
- **Agent Run Trace**: 一轮 Agent Message Run 的结构化总览，包含租户、用户、Agent、Session、Message、Run、Plan、状态、耗时、错误摘要和 artifact 引用。
- **Agent Trace Event**: Agent Runtime 执行过程中追加的单条事件，包含 node、phase、status、duration、input/output digest、error 和时间戳。
- **Agent Trace Node Snapshot**: 单个 Runtime 节点的结构化快照，包含节点输入摘要、输出摘要、上下文引用、模型/skill/tool 调用信息和错误详情。
- **Agent Run Report**: 面向 root 开发者下载的人读/机读报告，包含 Summary、User Message、Runtime Timeline、Intent/Planner、Skill/Tool Invocation、Final Response、Errors/Warnings。
- **Agent Run State**: 一轮 Message Run 的 UI 可渲染状态树，包含 run、session、message、response plan、tasks、agents、pending params、results、errors 和 trace links。
- **Agent Response Envelope**: 用户可见业务结果的统一结构，版本固定为 `powerx.agent.response/v1`，包含结果状态、对当前问题的回答、验收项、证据、缺口、下一步和 artifact 引用；它是 PowerX 渲染与持久化的输入，不是某个 Skill 的 Markdown 模板。
- **Agent Task State**: 单个任务节点的状态对象，关联 Agent、Skill、Capability、action、参数收集、结果、错误和 trace。
- **Plugin Agent Plugin Source**: 插件自有 Agent 开发态记录在 PowerX 侧的来源映射，包含插件 ID、插件 Agent ID、同步动作、底座 Agent UUID 和绑定 Skill 快照。
- **Plugin Skill Plugin Source**: 插件自有 Skill 开发态记录在 PowerX 侧的来源映射，包含插件 ID、插件 Skill ID、版本、manifest 快照、executor、capability 和 checksum。
- **Response Plan**: Agent Runtime 在最终回复前生成的结构化回答计划，决定 `response_mode`、目标能力、上下文层、是否需要澄清和最终回复模型。
- **Assistant Message Meta**: assistant 消息落库的结构化 metadata，包含 response mode、使用能力、上下文层、工具调用和模型选择，用于追问、去重和 Trace 分析。

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 管理员可在 10 分钟内完成一个 Skill 从导入到发布的完整流程，并可在异常时 5 分钟内完成回滚。
- **SC-002**: 95% 以上的合法 Skill 导入请求在 2 分钟内完成校验并进入草稿状态。
- **SC-003**: 双路径调用同一 Skill 时，99% 以上请求返回一致的业务状态语义与错误语义。
- **SC-004**: 授权不足或来源不可信的请求 100% 被阻断，且均可在审计中定位到拒绝原因。
- **SC-005**: 关键管理动作与调用事件 100% 具备可追溯记录，可按追踪标识在 3 分钟内完成定位。
- **SC-006**: 多租户环境下跨租户读取执行记录的成功率为 0%。
- **SC-007**: 在高基数候选场景下（单 Agent 10k 量级 Skill），单次匹配流程不得依赖全量线性扫描作为主路径。
- **SC-008**: 上线 Context 优化后，在稳定流量样本中，Agent 主入口 `prompt_tokens` P50 相对基线下降至少 30%。
- **SC-009**: 在支持缓存能力的模型上，固定前缀缓存命中率达到 60% 以上（以平台观测字段为准）。
- **SC-010**: 在 30 轮以上会话中，超上下文窗口错误发生率降至 1% 以下，且请求失败可通过 `trim_actions` 与 token 指标定位。
- **SC-011**: 在发布准备 A2A MVP 用例中（1 主 3 子 + 1 汇总），95% 请求可在目标 SLA 内返回完整结果或明确失败；演示团队任一子任务失败时必须停止下游节点，且子任务状态可按 `team_id/handoff_task_id/child_agent_id` 追溯。
- **SC-011a**: A2A seed 重复执行 3 次后，Agent、Skill、Binding、Team、TeamMember 记录数量保持稳定，且 latest published Skill 指针正确。
- **SC-012**: 团队任务验收中，100% 用例可在页面看到 `Intent + Plan + Node` 三段执行过程；不可见字段可在 3 分钟内通过审计接口定位。
- **SC-013**: 插件 Skill 发现后，95% 合法 Skill 在 3 分钟内被导入为草稿治理记录，非法 manifest 100% 被拒绝并返回明确错误。
- **SC-014**: 插件自有 Chat、Web Chat 与任一渠道入口触发同一插件 Skill 时，100% 走 PowerX Agent Session/Runtime，且 trace 中可看到相同的 `session_id/skill_id/plugin_id` 链路字段。
- **SC-015**: 缺少关键上下文或 capability 不匹配的插件 capability handler 调用 100% 被拒绝，且拒绝事件可在审计中按 `trace_id/plugin_id/skill_id` 检索。
- **SC-016**: Agent 主入口每轮消息 100% 生成 `run_id`，且成功或失败均可按 `tenant_uuid/session_id/message_id` 定位到 Agent Trace。
- **SC-017**: 本地开发模式下，95% 以上 Agent Run 在完成后 1 秒内生成 `run.json/timeline.jsonl/nodes/*.json`。
- **SC-018**: Root 用户可在 3 分钟内通过页面或 API 定位一轮 Message 的节点链路，并下载 Message 报告。
- **SC-019**: 非 root 用户访问 Agent Trace/Report 接口成功率为 0%，且 100% 返回 `AGENT_TRACE_ROOT_REQUIRED`。
- **SC-020**: 启用 Loki Sink 后，Agent Trace 事件 99% 可按 `run_id/message_id/node_kind/status` 在 Loki 中检索。
- **SC-021**: 插件自有 Agent/Skill 创建后，95% 合法同步请求在 3 秒内返回 PowerX 侧 `agent_uuid/skill_id` 与同步状态，非法绑定 100% fail-fast。
- **SC-022**: 插件调试页可运行 Agent 100% 来自已同步的 PowerX 底座 Agent 记录；未同步或同步失败的插件 Agent 不得进入 Agent Session 创建链路。
- **SC-023**: 用户询问当前 Agent 能力时，100% 回复仅包含当前 Agent 已绑定且可见的能力；未绑定的全局 system/public 候选不得出现在最终回答。
- **SC-024**: Agent Stream 中 100% 最终回复前产生 `response_plan` debug event，且 Message Trace 可定位到 `response_planner/context_builder/final_response` 节点。
- **SC-025**: 同一 session 连续询问能力时，系统 95% 以上可通过 assistant message meta 识别最近已介绍能力，并避免重复完整介绍。
- **SC-026**: 创建类任务缺少必填参数时，100% 在 PowerX Web Admin 与 PowerXPlugin 调试页展示 `awaiting_params` 状态和缺失字段，不得直接执行业务 capability。
- **SC-027**: 多 Agent 团队任务中，100% 用例可在页面看到主 Agent 与子 Agent task 状态，并可从任一失败 task 精确跳转到对应 Trace。
- **SC-028**: 页面刷新后，95% 以上已完成或失败的 Message Run 可从历史快照恢复 `AgentRunState`，不依赖重新执行 SSE。
- **SC-029**: 没有真实 task result 的业务执行请求，最终回复成功性误报率为 0。
- **SC-030**: 100% 执行、审核、发布和多 Agent 汇总结果通过 `powerx.agent.response/v1` 校验；非法 envelope 不得以成功或原始 Markdown 进入消息历史。
- **SC-031**: 对同一团队连续追问“某风险项具体检查什么”时，100% 最终答复直接覆盖当前问题，不重放无关的固定报告；实时 SSE 与刷新后的历史渲染结构一致。

## Assumptions

- 首版以 `SKILL.md` 作为唯一标准输入，其他格式后续扩展。
- 首版采用“受控导入 + 人工发布”作为默认治理路径，不允许绕过发布直接面向租户可见。
- 首版完整性门槛为 `checksum` 强制、`signature` 可配置强制。
- 首版第三方导入采用“上传 Bundle + 来源元数据登记”模式，仅记录来源，不执行在线拉取。
- 首版官方固有 Skills 目录由后端内置并随平台版本演进，不依赖外部实时同步。
- 首版对 Skill 的使用同时覆盖 Agent 主入口编排与 tenant 执行入口；两者共享相同治理与追踪语义。
- 首版来源策略遵循“请求上下文 > Agent 级 > 租户级 > 默认值”的优先级。
- 统一意图识别与编排以 LLM 决策为主，规则仅用于硬过滤与约束，不参与替代式主路由。
- Context 优化机制以“降成本与降延迟”为目标，不得改变既有授权边界、租户隔离和审计语义。
- A2A 协作首版只支持单租户内调度，不支持跨租户或跨组织边界的 Agent 任务委派。
- Agent Skill Bridge 归属本 feature；STS、Gateway、插件安装生命周期依赖 `007-integration-gateway-and-mcp` 与 `009-install-plugin-pxp` 的既有契约。
- 插件自有 Chat 是 PowerX Agent Session 的客户端，不是独立长期对话系统。
- Agent Run Trace & Report 归属本 feature 的 Agent Runtime 可观测扩展；首版以本地文件 sink 为必选 MVP，Loki sink 为生产目标能力。
- PowerXPlugin 插件 Agent/Skill Local 是开发态与声明源，不是运行态权威源；PowerX 底座 Agent/Skill/Binding 记录才是 Agent Runtime 权威源。
- Agent Response Planning 归属 Core Agent Runtime；插件只提供 Skill 源事实材料，不负责最终自然语言话术和上下文选择。
- 最终答复的区块、状态语义与 locale 渲染由 PowerX 平台统一治理；Skill/Agent 自定义只提供业务事实、业务字段说明、链接与素材，不得覆盖平台答复契约。
- Runtime Memory 不是 Agent 上下文权威源；任何影响权限、候选、message meta、会话历史或模型策略的上下文都必须可从 DB 权威记录恢复。
- Agent Run State Protocol 是 PowerX Runtime/UI/Trace 的内部状态协议，不替代 Google A2A；A2A handoff 节点必须映射为该协议中的 task 状态。
