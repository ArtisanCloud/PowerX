# Feature Specification: Unified Capability Contracts & Transport Adapters

**Feature Branch**: `002-title-unified-capability`  
**Created**: 2025-10-13  
**Status**: Draft  
**Input**: User description: "Title: Unified Capability Contracts & Transport Adapters
WHAT/WHY: Define a unified capability contract model (IDs, I/O schemas, error taxonomy) and transport adapters (HTTP/gRPC/MCP) so that all integration capabilities are described once and reused by consumers.
Scope: Contract schema, versioning, error model, transport adapter interfaces, compatibility rules.
Out-of-Scope: Registry, routing strategies, orchestration flows, gateway UI, business workflows.
Dependencies/Assumptions: Follows Integration Architecture baseline; Security & Tool Grants provide auth; EventBus provides topics if async needed."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - 一次声明全渠道复用的能力契约（Priority: P1）

作为平台集成负责人，我希望插件或服务提供者只需提交一份 Capability Contract，就能同时满足 MCP/gRPC/HTTP/Agent 等通道的需求，这样能力上线流程可以在 1 小时内完成并对所有调用方生效。

**Why this priority**: 没有统一契约就无法注册、发现或调用任何能力，是整个 Integration 体系的起点。

**Independent Test**: 仅部署契约模型与校验服务，通过提交新的能力描述并在三个传输通道中成功调用即可验证。

**Acceptance Scenarios**:

1. **Given** 提供者上传符合规范的 Capability Contract，**When** 契约通过校验写入真相源，**Then** 调用方可在 5 分钟内查询到能力元数据（ID、版本、IO、安全策略）。
2. **Given** 契约声明多个 Transport 选项，**When** 调用方分别以 gRPC 和 MCP 协议调用，**Then** 均能获取一致的输入输出校验与错误码描述。

---

### User Story 2 - 版本演进与兼容策略治理（Priority: P1）

作为产品运营人员，我需要在不影响现有调用的情况下升级能力版本、设置兼容策略或废弃提醒，以便管理多租户下的能力演进。

**Why this priority**: 版本治理直接决定企业是否敢于迭代能力；缺乏该能力会导致旧调用崩溃。

**Independent Test**: 单独上线版本管理模块，通过创建 v1/v2 契约、设置兼容标记并在调用时验证版本选择逻辑即可。

**Acceptance Scenarios**:

1. **Given** 存在已发布的 v1 契约，**When** 发布与之兼容的 v1.1 契约，**Then** 默认调用保持 v1.1，且可配置回退到 v1。
2. **Given** 契约被标记为 deprecated 且指定替代能力，**When** 调用方继续使用旧版本，**Then** 系统返回带替代建议的警告并记录审计日志。

---

### User Story 3 - 统一 Transport Adapter 接口（Priority: P2）

作为平台开发者，我希望所有 Transport Adapter 都遵循同一接口、上下文和错误模型，以便在新增协议或更新 QoS 策略时无需改动上层编排。

**Why this priority**: 统一接口大幅降低多协议维护成本，但优先级低于契约本身。

**Independent Test**: 单独构建 Adapter 规范，通过模拟请求调用 HTTP/gRPC/MCP/A2A 通道并验证统一上下文与错误码映射。

**Acceptance Scenarios**:

1. **Given** Adapter 接收标准化 TransportRequest，**When** 目标协议为 HTTP、gRPC、MCP，**Then** 返回的 TransportResponse/StreamChunk 均包含统一的 trace、状态与错误字段。
2. **Given** Adapter 在调用过程中遇到超时或重试，**When** 达到策略阈值，**Then** 产生的错误码映射到契约定义的 Error Taxonomy 并写入观测指标。

### Edge Cases

- 契约版本之间存在不兼容的 IO Schema 差异时，必须阻止自动升级，并提示兼容性检查失败。
- 提供者声明的传输通道缺少运行期支撑（例如未注册 MCP 会话），Adapter 需返回临时不可用错误且允许 Router 降级到其他通道。
- 契约引用的安全 Scope 或 Tool Grant 不存在时，系统需要拒绝发布并提供修复步骤。
- Adapter 遇到流式长连接中断时，要在 5 秒内尝试重连并保证调用链 trace 不丢失。

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: 平台必须提供 Capability Contract Schema，涵盖唯一 ID 命名规则、版本号、显示名称、描述、IO Schema 引用、安全策略与可观测配置。
- **FR-002**: 契约发布时必须执行 schema 校验、版本兼容性检查以及安全 Scope/ToolGrant 引用校验，校验失败需返回明确错误信息。
- **FR-003**: 系统必须支持契约的生命周期状态（draft/published/deprecated），并允许设置替代能力及生效时间。
- **FR-004**: 需提供 Error Taxonomy 定义，确保所有 Adapter 将底层错误映射到统一错误码与严重级别。
- **FR-005**: 版本管理必须支持向后兼容策略（默认选最新 minor 版本、允许固定 major 版本）并在调用上下文中传播所用版本。
- **FR-006**: Transport Adapter 接口需定义 Invoke、Stream、HealthCheck、Close 等方法，并支持同步与流式两种执行模型。
- **FR-007**: Adapter 必须处理统一的 TransportRequest/Response 结构，包括 trace_id、tenant、actor、超时、重试、stream 标记等核心字段。
- **FR-008**: Adapter 需提供策略化超时与重试控制（区分幂等与非幂等调用），并在达到阈值时返回契约定义的标准错误。
- **FR-009**: 平台必须允许为每个能力配置各传输通道的偏好（prefer/only/fallback），并在契约中持久化。
- **FR-010**: 系统需提供契约查询 API/SDK，供调用方按能力 ID、版本、传输偏好获取元数据与错误模型。
- **FR-011**: 契约发布或版本升级需触发审计日志与事件（含版本、发布者、影响范围）以便其他模块订阅。
- **FR-012**: Adapter 层必须输出统一的 metrics（调用耗时、错误率、活跃会话）和 tracing span，符合 Observability 规范。

### Key Entities *(include if feature involves data)*

- **CapabilityContract**: 描述能力的唯一标识、版本、显示信息、IO Schema 引用、安全策略、可观测配置与传输偏好，是所有调用的语义源头。
- **CapabilityVersionPolicy**: 定义版本兼容矩阵、默认策略、废弃状态与替代关系，供发布与路由决策使用。
- **IOSchemaDescriptor**: 记录输入/输出结构及验证规则，支持 JSON Schema/Protobuf 等引用形式。
- **TransportProfile**: 描述每个能力在不同协议下的执行策略（超时、重试、流式能力、prefer/fallback）。
- **ErrorTaxonomy**: 定义标准错误码、级别、分类及建议动作，Adapter 需映射到底层错误。
- **TransportRequest/Response**: 统一传输上下文数据结构，包含 trace、租户、actor、输入输出、状态和错误详情。

## Assumptions & Dependencies

- Registry/Router 提供契约持久化与选路能力，本特性只定义契约与 Adapter 接口。
- IAM 与 Tool Grant 子系统已暴露 Scope 校验接口，契约发布依赖其校验结果。
- EventBus 可用于发布契约变更与错误事件，供其他模块订阅。
- 插件 SDK/Agent 端会按照新契约格式上报能力与运行信息，不再自定义字段。

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 新增能力从提交契约到可调用的平均时间 ≤ 10 分钟，95% 的契约一次性校验通过。
- **SC-002**: 至少 90% 的调用在多协议切换时仍保持与契约一致的输入输出校验和错误码映射。
- **SC-003**: 契约版本升级后 48 小时内，未报告兼容性故障的租户比例 ≥ 99%。
- **SC-004**: Transport Adapter 统一接口上线后，新增协议接入平均耗时降低 40%，且调用链 tracing 覆盖率达到 100%。
- **SC-005**: 审计日志显示契约发布/升级事件可在 1 分钟内被其他模块订阅并处理，无丢失记录。
