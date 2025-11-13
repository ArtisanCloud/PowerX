# Feature Specification: Agent Lifecycle & Observability

**Feature Branch**: `008-agent-lifecycle-observability`  
**Created**: 2025-10-22  
**Status**: Draft  
**Input**: User description: "Agent Lifecycle & Observability: Manage agent registration, lifecycle (start/stop/scale), metrics/traces/logs across integration. Scope: Lifecycle API, health signals, metrics model, tracing, audit integration. Out-of-Scope: Orchestration logic; defining event fabric; defining grants. Dependencies: EventBus; Tool Grants; Contracts for telemetry schemas."

## Clarifications

### Session 2025-10-22

- Q: 在规范中提到的“默认容量阈值”，应以哪种度量方式定义，才最能指导生命周期扩缩容决策？ → A: 以可同时运行的代理实例/Pod 数量作为容量上限
- Q: 当健康评分触发“退化”告警时，系统应通过哪类主要渠道向值班团队发送通知？ → A: 通过企业即时通讯渠道群发（如 Slack、钉钉、企业微信）
- Q: 退役代理后，其历史指标与审计记录需要保留多长时间？ → A: 13 个月，与审计日志保留期一致

## Use Case Alignment

为满足跨场景对齐需求，`008-agent-lifecycle-observability` 在交付范围内落实以下 Use Case：

| Use Case | 关键诉求 | 本规范承诺 |
|----------|----------|------------|
| `UC-AGENT-REG-AUTO-001` | 插件启动时 5 秒内完成 manifest 自动注册、签名校验与沙箱准入 | 新增插件自动注册 Webhook、签名/Schema 校验、IAM 策略绑定与沙箱回执并写入生命周期档案 |
| `UC-AGENT-REG-TENANT-001` | 租户自助表单、策略校验、审批与沙箱激活 | 新增 Tenant Agent Center API、策略冲突引擎、审批工作流以及与生命周期服务共享的激活/审计通道 |
| `UC-AGENT-REG-LIFECYCLE-001` | 全量运行状态监控、僵尸治理、冻结/回收与审计 | 现有生命周期与健康引擎继续作为主干能力，扩展事件模型供其它场景消费 |
| `UC-AGENT-REG-SHARE-001` | 多租户共享、配额复制、验证与撤销 | 新增共享 API、Quota Provisioner、验证/撤销流程与事件/Audit 输出 |
| `UC-AGENT-REACT-THOUGHT-001` | ReAct 思考链需要可引用的 Agent 健康与授权上下文 | 输出标准化健康/审批快照 API 供 Thought Manager 检索与审计 |
| `UC-AGENT-REACT-ACTION-001` | Action Router 需实时校验代理状态、风险与 Trace | Lifecycle 控制面暴露低延迟状态/Trace 接口，并把审批/告警事件推入 `react.action` 订阅流 |
| `UC-AGENT-REACT-MEMORY-001` | Observation/记忆写回依赖代理健康、容量和 Loop 信号 | 将健康快照与 Loop Guard 指标对齐至 `react.loop.state`，并提供记忆写入审批勾子 |
| `UC-AGENT-REACT-AUDIT-001` | 30 秒内生成回放需完整生命周期与健康轨迹 | Lifecycle/Audit Schema 扩展 trace_id、事件引用，供 Playback 构建时间线 |
| `UC-AGENT-EXEC-PLAN-001` | 任务规划阶段需要代理能力/配额/健康基线 | 暴露用于 Planner 的查询接口，含容量、默认订阅与 SLA 元数据 |
| `UC-AGENT-EXEC-COORD-001` | DAG 调度需实时生命周期事件与阻塞告警 | 将状态事件流与阻塞/扩缩容动作同步到 StateBus，供协调器消费 |
| `UC-AGENT-EXEC-RECOVERY-001` | 失败恢复需要查询生命周期/告警记录并触发回滚 | 暴露恢复 API（冻结/解冻、退避参数）并把告警上下文注入 Copilot 工单 |
| `UC-AGENT-EXEC-CLOSURE-001` | 闭环验证需审计与健康摘要形成交付报告 | 提供闭环状态 API、审计链接与 13 个月保留策略，供报告/对账使用 |

## User Scenarios & Testing *(mandatory)*

### User Story 1 - 统一注册与启用代理 (Priority: P1)

作为平台管理员，我需要一次性提交代理的身份信息、所属租户、许可的 Tool Grants 以及遥测契约引用，以便在集成网关中安全上线新的代理能力。

**Why this priority**: 没有标准化注册就无法创建受控的代理目录，也无法绑定后续可观测与审计链路，直接阻断其它流程。

**Independent Test**: 仅部署生命周期登记接口，通过调用注册与激活流程，即可验证代理资料入库、依赖校验与默认监控基线是否建立。

**Acceptance Scenarios**:

1. **Given** 待接入的代理尚未在目录中存在，**When** 管理员提交包含身份、租户、Tool Grant 映射和遥测契约版本的注册请求，**Then** 系统创建代理档案、返回唯一标识并记录初始状态为“待激活”，同时生成审计条目与默认健康基线。
2. **Given** 代理已完成资料注册且所有依赖项可用，**When** 管理员发起“激活”操作并附带追踪标识，**Then** 系统校验依赖、将状态更新为“运行中”、发布生命周期事件并记录审计。

---

### User Story 2 - 按需调度代理容量 (Priority: P2)

作为运维调度负责人，我希望按租户或业务高峰动态启动、暂停或扩缩代理，以确保资源使用和服务质量在可控范围内。

**Why this priority**: 按需调整运行状态与容量是保障 SLA 的核心能力，直接影响到租户体验与成本。

**Independent Test**: 在测试环境中仅启用生命周期控制接口，通过发起 start/stop/scale 指令并观察状态机与事件流，即可独立验证功能。

**Acceptance Scenarios**:

1. **Given** 某代理当前处于“运行中”，**When** 运维人员发起“暂停”指令并提供原因，**Then** 系统验证无进行中的关键任务、将状态切换为“已暂停”、发布暂停事件并提示需人工恢复。
2. **Given** 某代理已设置容量上限，**When** 运维人员请求扩容至更高阈值，**Then** 系统校验扩容条件、更新容量配额、生成扩容事件并在历史中保留变更记录。

---

### User Story 3 - 端到端可观测与告警 (Priority: P3)

作为值班 SRE，我需要在一个界面中查看代理的健康评分、关键指标趋势、最近追踪与日志摘要，以便快速定位异常并触发处置。

**Why this priority**: 统一的可观测视角能够缩短故障定位时间，直接影响代理服务的可用性和恢复速度。

**Independent Test**: 通过模拟指标、追踪和日志输入，触发健康阈值并检查健康评分、告警通知、关联生命周期事件是否按期望输出，即可单独验证。

**Acceptance Scenarios**:

1. **Given** 代理正在运行且持续生成指标、追踪和日志，**When** 指标采集服务按 1 分钟节奏汇总吞吐、错误率与资源占用，**Then** 可观测视图展示最新健康评分、关键趋势和最后一次追踪入口供 SRE 检查。
2. **Given** 代理的错误率连续三个周期超过阈值，**When** 健康引擎判定为“严重”，**Then** 系统更新健康状态为“已退化”、发送告警通知并建议触发隔离或回滚操作。

---

### User Story 4 - 插件自动注册与沙箱准入 (Priority: P1)

作为插件 Guild 维护者，我希望插件在上线时通过 manifest Webhook 自动注册 Agent，系统需在 5 秒内完成签名/Schema 校验、IAM 策略绑定以及沙箱验证结果同步，并写入生命周期目录。

**Why this priority**: 平台超过 60% 的 Agent 来自插件随包自动注册，若缺少标准化流程将导致配额、授权与观测基线缺失，直接阻塞 `UC-AGENT-REG-AUTO-001`。

**Independent Test**: 仅启用自动注册 HTTP/gRPC 接口，通过发送 manifest（含签名）验证 Schema 校验、策略绑定、沙箱执行与审计事件。

**Acceptance Scenarios**:

1. **Given** 插件 webhook 提交包含签名、版本及能力声明的 manifest，**When** 注册接口校验通过，**Then** 系统生成 Agent ID、绑定 IAM 策略、触发沙箱验证并把过程写入审计。
2. **Given** manifest 签名无效或缺字段，**When** 校验失败，**Then** 系统拒绝注册、输出可调试错误码并触发告警，无任何脏数据写入。

---

### User Story 5 - 租户自助创建与审批 (Priority: P1.5)

作为租户管理员，我需要在 Tenant Agent Center 中配置自定义 Agent（用途、Prompt、引用插件、权限范围），系统应自动执行策略冲突检测、审批编排、沙箱激活，并将结果写入生命周期与可观测性链路。

**Why this priority**: 租户自建 Agent 是 `UC-AGENT-REG-TENANT-001` 的关键入口，若与生命周期能力割裂，将导致审批无法绑定运行态、审计不可追溯。

**Independent Test**: 通过租户 API 提交表单，模拟权限冲突、审批驳回与沙箱失败，验证状态机与审计记录。

**Acceptance Scenarios**:

1. **Given** 租户管理员提交完整表单，**When** 模板扫描、策略校验与审批全部通过，**Then** 系统生成 Agent ID、凭证与订阅配置，并把激活事件写入生命周期服务。
2. **Given** 表单存在权限冲突或审批逾期，**When** 状态机触发阻断，**Then** 系统返回冲突详情、保持 Agent 为 `pending`，并推送审批升级通知。

---

### User Story 6 - 多租户共享与撤销 (Priority: P2)

作为平台治理团队，我需要在 Agent Catalog 中将成熟 Agent 共享给其它租户，系统必须校验白名单、复制独立配额/凭证、执行租户验证，并支持一键撤销、审计与告警。

**Why this priority**: 共享能力是 `UC-AGENT-REG-SHARE-001` 的核心，缺少共享/撤销闭环会导致跨租户越权或资源泄露。

**Independent Test**: 通过共享 API 提交共享/撤销请求，验证白名单、配额复制、租户验证脚本与撤销告警路径。

**Acceptance Scenarios**:

1. **Given** 管理员为 Agent 配置共享请求并指定目标租户，**When** 白名单校验通过，**Then** 系统复制配额、生成独立凭证、触发租户验证并写入共享事件。
2. **Given** 共享租户验证失败或到期，**When** Compliance 引擎触发撤销，**Then** 系统收回凭证、释放配额、通知双方并写入审计。

---

### User Story 7 - ReAct & 任务执行可观测桥接 (Priority: P2)

作为 ReAct 编排与任务执行团队，我需要在思考/行动/协调/恢复/闭环过程中实时消费 Agent 生命周期、健康、审批与告警事件，以便在 30 秒内生成回放、调度 DAG，或触发 Copilot。

**Why this priority**: `SCN-AGENT-REACT-ORCH-001` 与 `SCN-AGENT-TASK-EXEC-001` 依赖生命周期服务提供可信遥测与恢复控制面，缺口会导致 ReAct 循环与任务执行无法合规运行。

**Independent Test**: 对接内部 StateBus，模拟 ReAct 与任务执行流程拉取生命周期/健康 API，验证事件延迟、Trace 关联与播放/回滚能力。

**Acceptance Scenarios**:

1. **Given** ReAct 思考链请求代理健康与审批快照，**When** 生命周期服务接收查询，**Then** 返回包含状态、容量、订阅与 Trace 的标准化文档，并附带审计引用供回放使用。
2. **Given** 任务执行引擎订阅 `agent.lifecycle.*` 事件，**When** 某节点被暂停、扩容或告警，**Then** 状态流在 <1 秒内推送到 StateBus，协调器可据此重排任务或触发 Copilot Handoff。

---

### Edge Cases

- 当管理员尝试使用已存在的代理别名或租户组合进行注册时，系统必须拒绝请求并返回冲突提示，同时保留原记录不变。
- 当生命周期指令与当前状态冲突（例如重复启动已运行的代理）时，系统需要阻止执行并告知当前状态及可用动作。
- 当连续两个采集周期未接收到代理的遥测数据时，系统应将健康状态标记为“未知”并提醒值班人员确认代理连通性。
- 当插件 manifest 签名失效或沙箱执行失败时，必须回滚所有 IAM/配额写入并仍保留完整审计记录供追查。
- 当共享 Agent 撤销过程中通知渠道不可用时，系统需自动重试并在 5 分钟内升级至值班渠道，防止凭证残留。
- 当 ReAct/任务执行循环订阅事件但生命周期服务检测到 Trace 缺失时，必须立刻阻断调用并提示人工补充 Trace。

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**：系统必须允许经过授权的管理员注册新代理，收集名称、唯一标识、所属租户、Tool Grant 清单、遥测契约版本、默认容量阈值（按可同时运行的代理实例/Pod 数量衡量）等必填属性。
- **FR-002**：系统必须在代理激活前校验依赖（EventBus 主题、遥测契约、Tool Grant 有效期），若任一缺失则返回明确的阻断原因并保持状态不变。
- **FR-003**：系统必须提供标准化的生命周期操作（启动、暂停、恢复、退役、扩容、缩容），并强制执行允许的状态转移矩阵以防止非法指令。
- **FR-004**：每次生命周期操作必须向 EventBus 发布事件，包含代理标识、目标状态、触发方、原因和追踪标识，以便下游消费。
- **FR-005**：系统必须维护代理生命周期历史与容量变更记录，至少保存 13 个月，可按代理与时间范围检索。
- **FR-006**：系统必须聚合指标、追踪和日志数据为单一健康评分，并在 60 秒内向用户呈现状态标签（健康、退化、不可用、未知）。
- **FR-007**：系统必须提供标准化的指标模型，至少覆盖吞吐量、请求成功率、P95 延时、资源占用，并支持按代理、租户与时间粒度（≥1 小时）检索。
- **FR-008**：当健康评分连续三个周期低于阈值时，系统必须自动标记为“退化”，通过企业即时通讯渠道推送通知，并将建议动作写入处置队列。
- **FR-009**：系统必须将所有注册、激活、生命周期和健康状态变更写入审计日志，记录操作者、时间、请求摘要与关联追踪 ID，保留不少于 13 个月。
- **FR-010**：系统必须允许管理员为代理配置可观测性订阅（例如特定指标或追踪过滤器），并在成功保存后即时生效且可回滚。
- **FR-011**：当代理退役后，系统必须继续保留其健康指标汇总与相关审计记录至少 13 个月，支持历史回溯与合规稽核。
- **FR-012**：系统必须提供插件自动注册 Webhook（HTTP+gRPC）、manifest Schema 校验、签名验证、IAM 策略绑定、沙箱验证与错误回滚能力，满足 `UC-AGENT-REG-AUTO-001` 的 5 秒 SLA 与审计要求。
- **FR-013**：系统必须提供租户自建 Agent 控制面（表单、策略冲突检测、审批状态机、沙箱激活），并与生命周期目录、告警与审计保持一致，满足 `UC-AGENT-REG-TENANT-001`。
- **FR-014**：系统必须实现多租户共享/撤销 API，覆盖白名单校验、配额复制、租户验证、合规撤销与审计通知，满足 `UC-AGENT-REG-SHARE-001`。
- **FR-015**：系统必须把生命周期、健康、审批与告警事件以标准化 Schema 推送到 StateBus，供 `UC-AGENT-REACT-THOUGHT/ACTION/MEMORY/AUDIT` 消费，并提供按 Trace 查询接口。
- **FR-016**：系统必须向 `UC-AGENT-EXEC-PLAN/COORD/RECOVERY/CLOSURE` 暴露查询与控制 API（容量、状态变更、冻结/解冻、闭环摘要），并与 Copilot/Workflow 打通以执行回滚与闭环审计。

### Key Entities *(include if feature involves data)*

- **AgentProfile**：代表可在集成网关运行的代理，包含身份信息、租户归属、Tool Grant 清单、遥测契约版本、默认容量阈值和当前生命周期状态。
- **LifecycleEvent**：记录每一次生命周期指令或自动状态变更，追踪触发方、目标状态、理由、时间戳与关联事件编号。
- **HealthSignalSnapshot**：汇总某一时间点的指标、追踪、日志摘要和计算出的健康评分，用于驱动状态展示与告警。
- **AuditRecord**：详细保存敏感操作的执行情境，与 AgentProfile 及 LifecycleEvent 关联，支持合规稽核与可追溯。

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**：95% 的新代理在提交注册后的 15 分钟内完成激活并进入监控视图。
- **SC-002**：99% 的生命周期指令在 30 秒内完成状态更新并成功发布对应事件。
- **SC-003**：90% 的健康异常在发生后的 2 分钟内被检测并通知到值班团队。
- **SC-004**：100% 的生命周期和健康状态变更可在审计查询中于 10 秒内返回完整记录。
- **SC-005**：插件自动注册成功率 ≥98%，manifest 校验与沙箱回执 p95 <5 秒，失败全部具备审计溯源。
- **SC-006**：租户自助 Agent 审批平均耗时 <2 个工作日，权限冲突阻断率 100%，沙箱验证成功率 ≥95%。
- **SC-007**：跨租户共享/撤销请求 1 分钟内完成 ≥95%，撤销失败率 <1%，共享事件 100% 写入审计。
- **SC-008**：ReAct 与任务执行场景订阅到的生命周期/健康事件延迟 <1 秒，Trace 丢失率 0%，回放生成成功率 100%。

## Assumptions & Dependencies

- 假设 EventBus 已提供高可用主题，且允许发布新的生命周期事件类型。
- 假设 Tool Grant 管理服务已能返回代理可用的授权清单，并由其负责授权内容的准确性。
- 假设遥测契约（指标、追踪、日志模式）已有版本化合同，可在注册时引用并在后续保持向后兼容。
- 功能范围不涵盖编排逻辑、事件织构或 Grant 体系的定义，本规范仅消费这些能力。
- 插件构建链路可在注册时提供签名 manifest 与沙箱脚本，Tenant Agent Center 与 Agent Catalog 已具备基础 UI/工作流能力，可直接接入生命周期服务。
