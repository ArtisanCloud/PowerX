# Tasks: 通用 WebSocket 消息总线

**Input**: Design documents from `/specs/012-websocket-docs-plan/`
**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md, contracts/

**Tests**: The feature spec does not require tests explicitly, but contract tests for WS entry and integration checks for user stories are included as [P] where applicable.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`
- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

## CoreX Gate Compliance (Non-CRUD WS Enhancement)

- [X] T-COREX-EX-001 记录双传输豁免依据并与 spec 对齐 `specs/012-websocket-docs-plan/spec.md`, `specs/012-websocket-docs-plan/plan.md`
- [X] T-COREX-EX-002 校验 WS 入口合同为唯一 REST 设计产物 `specs/012-websocket-docs-plan/contracts/http-openapi.yaml`
- [X] T-COREX-EX-003 确认无需新增 proto/生成链路与 gRPC server 装配（仅文档说明）`specs/012-websocket-docs-plan/plan.md`

## Phase 1: Setup (Shared Infrastructure)

- [X] T001 [P] [US1] 创建通用 WS 通道的 OpenAPI 入口文档（已存在则校验）`specs/012-websocket-docs-plan/contracts/http-openapi.yaml`
- [X] T002 [P] [US1] 补充入库任务进度结构与阶段定义文档（如需更新）`specs/012-websocket-docs-plan/data-model.md`

---

## Phase 2: Foundational (Blocking Prerequisites)

- [ ] T003 [US1] 实现通用 WS Hub（连接管理、topic 订阅/取消、消息分发）`backend/internal/transport/websocket/`
- [ ] T004 [US1] 添加租户绑定与鉴权校验（连接建立与订阅时）`backend/internal/transport/websocket/`
- [ ] T005 [US1] 增加入库进度事件发布入口（service 层统一发布）`backend/internal/service/knowledge_space/`
- [ ] T006 [US2] 统一 WS 消息 envelope 定义与序列化结构（后端与前端一致）`backend/internal/transport/websocket/`, `web-admin/app/composables/`
- [ ] T006A [US2] 形成消息结构/schema 说明并对齐合同文档 `specs/012-websocket-docs-plan/contracts/http-openapi.yaml`

**Checkpoint**: Hub 能建立连接、订阅/取消订阅、发布消息；tenant 绑定完成。

---

## Phase 3: User Story 1 - 入库进度实时可见 (Priority: P1)

**Goal**: 入库任务状态/进度实时推送，页面无需刷新。

**Independent Test**: 触发入库任务后观察进度条与状态实时更新。

### Tests for User Story 1

- [ ] T007 [P] [US1] 合同校验：WS 连接入口返回 101（文档一致性检查）`specs/012-websocket-docs-plan/contracts/http-openapi.yaml`
- [ ] T008 [P] [US1] 集成验证：触发入库任务后实时进度可见（手工/脚本验证）`specs/012-websocket-docs-plan/quickstart.md`

### Implementation for User Story 1

- [ ] T009 [US1] 入库 pipeline 写入阶段性进度并推送（extract/chunk/embed/persist）`backend/internal/service/knowledge_space/ingestion_service.go`
- [ ] T010 [US1] WS topic：`knowledge.ingestion.job` 推送入库状态与进度 `backend/internal/transport/websocket/`
- [ ] T011 [US1] web-admin 接入 WS 并替换轮询为主通道（轮询保留兜底）`web-admin/app/composables/`, `web-admin/app/pages/knowledge-spaces/`
- [ ] T012 [US1] 前端进度节流：最多 1 秒更新一次 `web-admin/app/pages/knowledge-spaces/`

---

## Phase 4: User Story 2 - 单连接承载多模块通知 (Priority: P2)

**Goal**: 单连接多主题订阅，多个模块共享 WS。

**Independent Test**: 同一页面订阅多个 topic，只建立一个连接。

### Tests for User Story 2

- [ ] T013 [P] [US2] 集成验证：多个 topic 共用一条连接（浏览器连接数检查）`specs/012-websocket-docs-plan/quickstart.md`
- [ ] T013A [P] [US2] 校验无权限订阅被拒绝（后端返回错误并不推送）`specs/012-websocket-docs-plan/quickstart.md`

### Implementation for User Story 2

- [ ] T014 [US2] 前端 WS 客户端集中管理订阅与分发 `web-admin/app/composables/`
- [ ] T015 [US2] 订阅/取消订阅协议与权限拒绝逻辑 `backend/internal/transport/websocket/`

---

## Phase 5: User Story 3 - 断线回退与恢复 (Priority: P3)

**Goal**: WS 断线时回退轮询，恢复后自动继续订阅。

**Independent Test**: 断开网络后仍看到状态更新；恢复后继续实时推送。

### Tests for User Story 3

- [ ] T016 [P] [US3] 集成验证：断线回退与恢复订阅 `specs/012-websocket-docs-plan/quickstart.md`

### Implementation for User Story 3

- [ ] T017 [US3] 自动重连与租户切换重连（清空旧订阅）`web-admin/app/composables/`
- [ ] T018 [US3] 断线时启用轮询兜底并提示状态 `web-admin/app/pages/knowledge-spaces/`

---

## Phase N: Polish & Cross-Cutting Concerns

- [ ] T019 [P] 文档更新（若实现细节改变）`specs/012-websocket-docs-plan/*.md`
- [ ] T020 [P] 性能与日志检查：入库进度推送不影响主流程，记录基础指标与日志 `backend/internal/service/knowledge_space/`, `backend/internal/transport/websocket/`
- [ ] T020A [P] 添加 WS Hub/订阅基础测试并记录覆盖率要求 `backend/internal/transport/websocket/`
- [ ] T020B [P] 追踪/指标覆盖校验（trace_id、tenant_id、p95 记录）并记录最低覆盖率门槛 `backend/internal/transport/websocket/`, `backend/internal/service/knowledge_space/`

---

## Dependencies & Execution Order

- Phase 2 阻塞所有用户故事实现。
- US1 在 Phase 2 完成后优先实施。
- US2、US3 可在 US1 完成基础能力后并行。
- 断线回退需依赖前端 WS 客户端存在。

## Parallel Example: User Story 1

```bash
Task: "合同校验：WS 连接入口返回 101（文档一致性检查）"
Task: "集成验证：触发入库任务后实时进度可见"
```
