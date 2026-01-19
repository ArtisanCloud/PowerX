# Feature Specification: 通用 WebSocket 消息总线

**Domain Ownership**: CoreX (`corex.knowledge`)

**Feature Branch**: `012-websocket-docs-plan`  
**Created**: 2026-01-19  
**Status**: Draft  
**Input**: User description: "通用 WebSocket 消息总线方案：基于 docs/plan/wx/WS-NOTIFY.md 生成规格说明"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - 入库进度实时可见 (Priority: P1)

作为使用知识入库的用户，我希望看到任务进度和状态实时更新，不需要刷新页面，从而能清楚任务是否卡住、完成或失败。

**Why this priority**: 这是最核心的用户体验改进，直接影响入库可用性与信任度。

**Independent Test**: 仅实现入库任务的实时推送即可验证；用户触发入库后无需刷新即可看到进度与状态变化。

**Acceptance Scenarios**:

1. **Given** 用户在入库页面且任务处于运行中，**When** 后端进度推进，**Then** 页面在不刷新情况下展示新的进度与状态。
2. **Given** 任务完成或失败，**When** 状态变更发生，**Then** 页面立即显示最终状态与完成/失败信息。

---

### User Story 2 - 单连接承载多模块通知 (Priority: P2)

作为系统使用者，我希望所有异步通知共享同一条连接，避免多个模块各自建立连接导致资源浪费与状态不一致。

**Why this priority**: 单连接降低资源占用，提升稳定性，是平台级能力。

**Independent Test**: 仅实现通用连接与订阅/取消订阅即可验证；多个模块消息走同一连接。

**Acceptance Scenarios**:

1. **Given** 用户已建立通用连接，**When** 订阅多个主题，**Then** 只使用一条连接即可收到多个主题的消息。

---

### User Story 3 - 断线回退与恢复 (Priority: P3)

作为用户，我希望当实时连接不可用时系统能继续显示任务状态，并在连接恢复后继续接收消息。

**Why this priority**: 保证稳定性与可用性，避免单点失败导致信息缺失。

**Independent Test**: 仅实现断线回退逻辑即可验证；在断线情况下仍能看到状态更新。

**Acceptance Scenarios**:

1. **Given** 连接断开，**When** 用户继续查看入库任务，**Then** 页面仍可通过回退机制看到状态更新。

---

### Edge Cases

- 当用户切换租户时，如何避免收到旧租户的消息？
- 当用户订阅了主题但无权限访问对应资源时，系统如何处理？
- 当短时间内推送大量进度更新时，前端如何避免显示抖动？

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: 系统必须提供一个通用实时消息连接，所有模块共享该连接。
- **FR-002**: 用户必须能够订阅与取消订阅指定主题，且订阅范围仅限其权限范围。
- **FR-003**: 系统必须支持为入库任务推送状态与进度更新，用户无需刷新即可看到变化。
- **FR-004**: 系统必须在连接不可用时提供回退方式保证状态可见。
- **FR-005**: 系统必须确保不同租户之间的消息隔离，避免跨租户推送。
- **FR-006**: 系统必须为所有消息提供统一的消息结构，以便前端统一处理。
- **FR-007**: 当用户切换租户时，系统必须断开并重连实时连接，且清空旧订阅。
- **FR-008**: 当用户订阅无权限主题时，系统必须拒绝订阅并返回错误。
- **FR-009**: 前端必须对进度推送进行节流，最多每 1 秒更新一次展示，避免界面抖动。

### Non-Functional Requirements

- **NFR-001**: WS 连接与入库进度推送需满足单页面 10k 并发连接的部署目标（按环境可调）。

### Transport Exemption

本特性为 **非 CRUD 的传输层增强**，仅新增通用 WebSocket 通道与消息推送能力；不新增领域实体与 CRUD API。

- **REST**: 仅提供 WS 握手入口的 OpenAPI 合同（`specs/<feature>/contracts/http-openapi.yaml`）。
- **gRPC**: 不新增 gRPC 服务或 proto，本特性不适用双传输要求（仅限本次 WS 传输增强范围）。

### Key Entities *(include if feature involves data)*

- **实时消息连接**: 用户与系统之间的持续连接，包含当前订阅主题与租户上下文。
- **主题订阅**: 订阅的主题列表与用户权限绑定。
- **消息事件**: 系统推送的事件单元，包含主题、类型与业务载荷。
- **入库进度**: 任务的状态、阶段与进度百分比等信息。

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 入库任务状态在变更后 2 秒内显示到页面（无需刷新）。
- **SC-002**: 用户浏览入库页面时只建立 1 条实时连接。
- **SC-003**: 连接断开时页面仍可在 10 秒内显示最新任务状态（通过回退机制）。
- **SC-004**: 不同租户用户不会收到其他租户的任务消息（0 起跨租户消息泄漏）。

## Assumptions

- 默认以任务状态和进度为首批推送内容，其他模块后续接入。
- 前端在连接不可用时继续使用现有轮询作为回退方式。
- 入库进度以阶段性百分比展示，不需要逐条 chunk 级别实时更新。

## Clarifications

### Session 2026-01-19

- Q: 租户切换时 WS 连接如何处理？ → A: 切换租户时强制断开并重连，清空旧订阅。
- Q: 当用户订阅了无权限主题时，系统应如何处理？ → A: 返回错误消息并拒绝订阅。
- Q: 高频进度更新时，前端如何避免显示抖动？ → A: 前端节流，最多每 1 秒更新一次。
