# Feature Specification: PowerX Skills 管理与治理

**Feature Branch**: `024-ai-engineering-skills`  
**Created**: 2026-03-09  
**Status**: Draft  
**Input**: User description: "请根据 docs/plan/ai_engineering/skills 的开发计划文档，生成对应的 spec 文档"

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

### Edge Cases

- 当同一 `skill_id` 同时存在多个可用版本时，如何明确“当前生效版本”并避免歧义调用？
- 当 Skill 已绑定能力后被停用或下线时，系统如何避免出现悬空调用入口？
- 当执行处于安全模式且 Skill 被判定为高风险时，系统如何阻断执行并给出可操作反馈？
- 当导入来源发生变更但版本号未变时，系统如何防止隐式覆盖和来源漂移？
- 当调用链路中出现临时失败时，系统如何区分可重试与不可重试错误，避免错误重试放大影响？

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

### Key Entities *(include if feature involves data)*

- **Skill Registry Record**: Skill 的注册与治理记录，包含标识、版本、来源、状态、清单快照、完整性信息和绑定关系。
- **Skill Execution Trace**: 一次调用的执行追踪记录，包含追踪标识、租户、Skill、版本、入口、状态、时延与错误摘要。
- **Skill Source Descriptor**: Skill 来源描述，包含来源类型、来源地址、来源版本标识与导入操作者信息。
- **Capability Binding**: Skill 与统一能力入口之间的映射关系，包含可见性和授权约束。
- **Skill Lifecycle Event**: 生命周期关键动作事件，包含导入、发布、回滚、停用等动作及其操作者与时间。

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 管理员可在 10 分钟内完成一个 Skill 从导入到发布的完整流程，并可在异常时 5 分钟内完成回滚。
- **SC-002**: 95% 以上的合法 Skill 导入请求在 2 分钟内完成校验并进入草稿状态。
- **SC-003**: 双路径调用同一 Skill 时，99% 以上请求返回一致的业务状态语义与错误语义。
- **SC-004**: 授权不足或来源不可信的请求 100% 被阻断，且均可在审计中定位到拒绝原因。
- **SC-005**: 关键管理动作与调用事件 100% 具备可追溯记录，可按追踪标识在 3 分钟内完成定位。
- **SC-006**: 多租户环境下跨租户读取执行记录的成功率为 0%。
- **SC-007**: 在高基数候选场景下（单 Agent 10k 量级 Skill），单次匹配流程不得依赖全量线性扫描作为主路径。

## Assumptions

- 首版以 `SKILL.md` 作为唯一标准输入，其他格式后续扩展。
- 首版采用“受控导入 + 人工发布”作为默认治理路径，不允许绕过发布直接面向租户可见。
- 首版完整性门槛为 `checksum` 强制、`signature` 可配置强制。
- 首版第三方导入采用“上传 Bundle + 来源元数据登记”模式，仅记录来源，不执行在线拉取。
- 首版官方固有 Skills 目录由后端内置并随平台版本演进，不依赖外部实时同步。
- 首版对 Skill 的使用同时覆盖 Agent 主入口编排与 tenant 执行入口；两者共享相同治理与追踪语义。
- 首版来源策略遵循“请求上下文 > Agent 级 > 租户级 > 默认值”的优先级。
- 统一意图识别与编排以 LLM 决策为主，规则仅用于硬过滤与约束，不参与替代式主路由。
