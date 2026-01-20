# Implementation Plan: 通用 WebSocket 消息总线

**Branch**: `012-websocket-docs-plan` | **Date**: 2026-01-19 | **Spec**: `/private/var/www/html/ArtisanCloud/X/PowerX/Core/PowerX/specs/012-websocket-docs-plan/spec.md`
**Input**: Feature specification from `/specs/012-websocket-docs-plan/spec.md`

## Summary

为 PowerX 提供单连接、多主题的通用 WebSocket 消息总线，用于入库任务进度实时推送，并保留轮询回退；同时落地通知持久化与 WS 实时推送能力；实现租户切换重连、无权限订阅拒绝、前端节流展示。

## Technical Context

**Language/Version**: Go 1.24（backend），Node 20 + Nuxt 4（web-admin）  
**Primary Dependencies**: Gin HTTP 栈、gorilla/websocket、Pinia、Nuxt UI  
**Storage**: PostgreSQL（ai_model_profiles/knowledge_*），Redis（现有队列/缓存）  
**Testing**: Go `go test`（backend），Playwright（web-admin 既有）  
**Target Platform**: Linux server + 现代浏览器  
**Project Type**: Web application（backend + web-admin）  
**Performance Goals**: 进度更新可见延迟 ≤ 2 秒；单页面仅 1 条 WS 连接  
**Constraints**: 多租户隔离；断线回退 ≤ 10 秒；无权限订阅拒绝  
**Scale/Scope**: 支持 10k 并发连接的通用消息分发（按部署规模可调）

## Constitution Check

- **CoreX Module**: 本功能属于 CoreX 平台层能力（通用通知/任务进度），不走插件机制。**PASS**
- **Dual Transport**: 本功能为通用 WS 传输增强，不新增 CRUD 领域服务；已提供 WS 入口的 HTTP 合同文档（OpenAPI）。gRPC 不新增属于非 CRUD 传输场景，列为本次豁免范围（已在 spec 明确）。**PASS（WS 非 CRUD 增强）**
- **Multi-tenant**: 订阅与消息投递必须绑定租户上下文。**PASS**
- **Observability**: 需要结构化日志与基础指标。**PASS**

## Ruleset Alignment (Constitution Gates)

**Ruleset Alignment (explicit):**
- **@dev-crud-http**: 本特性仅提供 WS 握手入口的 OpenAPI 合同（`specs/<feature>/contracts/http-openapi.yaml`），不新增 CRUD HTTP 资源；其余 CRUD 规则项不适用。**PASS（限定范围）**
- **@dev-crud-grpc**: 本特性为 WS 传输增强，不新增 gRPC 合同/实现；该规则集在本特性范围内豁免。**EXEMPT**

**Gate Checks:**
- **HTTP_PRESENT**: 仅提供 WS 握手入口的 OpenAPI 合同（`specs/<feature>/contracts/http-openapi.yaml`）。**PASS**
- **GRPC_PRESENT**: 已在 spec 明确豁免，不新增 gRPC 合同/实现。**EXEMPT**
- **PROTOBUF_DEFINED**: 同上豁免。**EXEMPT**
- **SERVER_DEFINED**: 同上豁免。**EXEMPT**
- **MAKE_TARGETS**: 未新增 proto 生成链路，本特性不涉及。**EXEMPT**

## Project Structure

### Documentation (this feature)

```
specs/012-websocket-docs-plan/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
└── tasks.md
```

### Source Code (repository root)

```
backend/
├── internal/transport/websocket/
├── internal/transport/http/admin/knowledge_space/
├── internal/transport/http/admin/notifications/
├── internal/service/knowledge_space/
├── internal/service/notifications/
├── internal/service/agent/
├── internal/bootstrap/app.go
└── internal/http/router.go

web-admin/
├── app/composables/
├── app/stores/
└── app/pages/knowledge-spaces/
```

**Structure Decision**: 该功能为 CoreX 通用传输能力，后端落地在 `internal/transport/websocket` 与相关服务层；前端落地在 `web-admin/app/composables` 与入库页面。

## Complexity Tracking

无额外复杂度违例。

## Phases

1. **WS 基础设施**：Hub、鉴权、租户绑定、消息 envelope。
2. **入库进度推送**：服务端发布、前端订阅与节流展示。
3. **通知持久化 + WS 推送**：通知表/接口/列表接入，总线推送。
4. **断线回退**：轮询兜底 + 自动重连/租户切换重连。
5. **校验与观测**：手工验证、日志与指标检查。

## Notification Data Flow

1. 后端写入通知记录（含租户/成员维度）。
2. 写库成功后通过 WS 总线推送 `system.notification`。
3. 前端通知列表接收 WS 增量并与列表接口对齐。
