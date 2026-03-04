# OpenAPI API Key 独立特性计划（仅开放 OpenAPI，禁止 Admin Key 直调）

## 文档定位

- 目录：`docs/plan/integration/`
- 原因：该特性属于对外集成能力与认证边界调整，和 `powerx_capability.md` 同域。

## 背景

当前 PowerX 插件与外部调用主要依赖 Access Token（JWT）贯穿。随着远程调试与多平台 OpenAPI 开放需求增加，需要引入类似 OpenAI 的 API Key 调用体验。

但需要严格保证：

1. OpenAPI / tenant 业务接口可以使用 API Key。
2. Admin 管理接口不能通过 API Key 访问，仍仅允许 JWT + RBAC。

## 目标

1. 支持平台动态签发 `pxk_` 前缀 API Key 给第三方系统。
2. API Key 服务端持久化存储（仅存哈希，不存明文）。
3. OpenAPI 业务接口支持 `Authorization: Bearer <api_key>`。
4. Admin 接口维持 JWT 鉴权，不接受 API Key。
5. 保持现有插件 Access Token 调用链兼容。

## 非目标

1. 不把 API Key 用于 Admin API 登录或管理操作。
2. 不替换现有 JWT 体系（先做并行双通道：JWT 或 API Key）。
3. 不在本阶段引入复杂多级审批流（先提供创建、列表、吊销、轮换基础能力）。

## 现状基线（代码）

1. Admin 与 OpenAPI 当前共用 JWT 中间件：
   - `backend/internal/http/router.go`
   - `backend/internal/transport/http/admin/routes.go`
   - `backend/internal/transport/http/openapi/routes.go`
2. JWT 中间件当前仅解析 Bearer JWT：
   - `backend/pkg/auth/middleware/jwt.go`
3. 已存在 API Key 基础模型与仓储（可复用）：
   - `backend/pkg/corex/db/persistence/model/iam/api_key_gorm.go`
   - `backend/pkg/corex/db/persistence/repository/iam/api_key_repo.go`

## 设计原则

1. **边界隔离**：Admin 与 OpenAPI 认证链路明确分离。
2. **最小侵入**：优先复用已有 IAM 模型与仓储。
3. **安全默认**：明文只展示一次、哈希存储、可吊销、可审计。
4. **渐进迁移**：先兼容 JWT，再逐步引导第三方改用 API Key。

## 目标鉴权模型

### Admin 路由

1. 仅接受 JWT（现有逻辑延续）。
2. API Key 一律拒绝（返回 401/403）。

### OpenAPI 路由

1. 支持 JWT（兼容现有插件/租户调用）。
2. 支持 API Key（面向第三方/跨平台）。
3. 统一落到租户上下文与 scope 校验。

## 数据模型（演进建议）

在现有 `iam_api_key` 基础上扩展：

1. `key_prefix`：用于展示与检索（如 `pxk_abcd...` 前缀）。
2. `scopes`：允许调用范围（如 `llm.invoke`, `media.assets.read`）。
3. `status`：active / disabled / revoked。
4. `expires_at_ms`：可选过期时间。
5. `created_by`：审计追踪。
6. `rotation_of`：轮换链路。

> 备注：`key_hash` 保持唯一；明文 key 仅创建时返回。

## 接口草案

### Admin 管理 API（仅 JWT + RBAC）

1. `POST /admin/api-keys`：创建 key（返回一次明文）。
2. `GET /admin/api-keys`：列表（不返回明文）。
3. `DELETE /admin/api-keys/:id`：吊销 key。
4. `POST /admin/api-keys/:id/rotate`：轮换 key（可选同 scope 继承）。

### OpenAPI 业务 API（JWT 或 API Key）

1. 保持既有路径不变，如：
   - `/tenant/capabilities`
   - `/tenant/invocations`
   - `/ai/*`
2. 认证层支持 Bearer 中的 JWT 或 `pxk_` Key。

## 分阶段实施清单

### Phase 1：最小可用（MVP）

1. 新增 `OpenAPIKeyMiddleware`（仅挂 OpenAPI 组）。
2. 新增 API Key 创建/列表/吊销 Admin API。
3. OpenAPI 路由改为“JWT 或 API Key”双通道。
4. 增加基础审计日志（create/revoke/use）。

### Phase 2：安全与治理增强

1. 增加 scope 校验。
2. 增加过期时间、状态管理、轮换能力。
3. 增加 `last_used_at` 异步回写与频控。

### Phase 3：文档与契约对齐

1. 更新 OpenAPI 契约文档：明确支持 API Key principal。
2. 更新集成指南：区分 Admin JWT 与 OpenAPI API Key。
3. 输出第三方接入示例（curl/SDK）。

## 验收标准

1. 使用 API Key 调用 OpenAPI 成功（200），可命中租户隔离与 scope。
2. 使用 API Key 调用任意 `/admin/*` 必须失败（401/403）。
3. 使用现有 JWT 的插件调用链路保持可用。
4. 数据库仅存哈希，不可反推明文。
5. 吊销后 key 立即失效。

## 风险与缓解

1. **风险**：OpenAPI 与 Admin 鉴权混用导致越权。
   - **缓解**：中间件按路由组分层挂载，增加回归测试覆盖“API Key 调 admin”。
2. **风险**：scope 映射不完整导致误拒绝。
   - **缓解**：先提供保守默认 scope + 白名单放行策略，并配套观测。
3. **风险**：高并发下 `last_used_at` 写放大。
   - **缓解**：异步/限频更新（如分钟级批量更新）。

## 建议任务拆分（开发工单）

1. IAM 模型扩展与迁移。
2. API Key 应用服务（签发、校验、吊销、轮换）。
3. OpenAPI 认证中间件改造（JWT/API Key 双通道）。
4. Admin API Key 管理接口。
5. 契约文档与测试用例补齐。

## 与既有规划关系

该特性为 `docs/plan/integration/powerx_capability.md` 的认证能力增强子项，可作为独立 Feature 并行推进，不改变既有能力目录与路由主干。
