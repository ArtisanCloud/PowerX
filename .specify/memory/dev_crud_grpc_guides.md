# PowerX CRUD – gRPC Guides

> 本文与《dev_crud_http_guides.md》并列，复用同一套 **Model / Repository / Service** 主干，仅替换传输层为 gRPC。
> 入口仍由 `constitution.md` 控制，别名解析由同级 `manifest.yaml` 完成（`@dev-crud-grpc`）。

---

## 1. 适用范围（Scope）

* 与 HTTP 版 CRUD 规范**等价**：创建/查询/更新/删除、分页、错误、审计、多租户。
* 仅规定 **gRPC 传输层**：Server 行为、拦截器、错误映射、分页/上下文 DTO 对齐 **proto 契约**。
* 宪章入口仍为 `.specify/memory/constitution.md`（由其 `use: "` 决定是否引入本指南）。"

---

## 2. 目录与生成物（与你仓库对齐）

```
api/grpc/
  contracts/                # *.proto 契约（Buf 维护）
    buf.yaml
    buf.gen.yaml
    common/v1/{context,pagination,resource,response}.proto
    powerx/{iam,agent,auth/sts,plugin/control,setting}/v1/*.proto
  gen/go/...                # 代码生成产物（勿手改）
  sdk/{ts,rust,php}/...     # 多语言 SDK（封装拨号、上下文、重试等）

internal/transport/grpc/
  iam/{member_handler.go,team_handler.go,...}
  agent/{stream_handler.go,setting_handler.go,...}
  auth/{sts_handler.go}
  auth/middleware/auth_interceptor.go
```

* 你的 `api/grpc` 与 `internal/transport/grpc` 已经是成熟形态；本指南只统一**约束与验收**。

---

## 3. 设计原则（Transport-Agnostic）

* **服务端不写业务**：gRPC Server 仅做 **参数绑定/校验 → 调 Service → 错误映射**。
  示例：`MemberServer` 只解析上下文、分页、调用 `MemberService.ListMembers` 并映射返回。
* **多租户一致**：`tenant_id` 必须来自 **RequestContext 或 Metadata**；缺失返回 400 语义。
* **错误等价**：将应用错误（`ErrInvalidParam/ErrForbidden/ErrNotFound/ErrConflict`）映射为 gRPC `codes.*`；HTTP 与 gRPC 语义一致。
* **分页一致**：`common.v1.pagination.proto` 的 PageRequest/PageResponse 必须与 HTTP 页码/页大小语义一致。
* **审计与鉴权**：通过拦截器（Auth + Tenant）注入上下文，Service 侧照常记录审计；Server 不直接写审计逻辑。

---

## 4. Proto 契约（最小统一约束）

* **公共类型**：统一使用 `common/v1` 下的 `context.proto / pagination.proto / resource.proto / response.proto`。
* **包与版本**：采用 `powerx.<domain>.v1`，破坏性变更才升级 `v2`。
* **分页**：推荐结构（与你现有 common 保持一致）：

    * `PageRequest{ page, page_size, sort_by, sort_order, offset? }`
    * `PageResponse{ total, page, page_size, pages? }`
* **Context**：`RequestContext{ tenant_id, request_id, actor_id, trace_id, ... }`，所有 RPC **强制**携带。
* 生成配置使用 Buf：`buf.yaml / buf.gen.yaml`（已存在）。

> 你现有 `powerx/iam/v1/member.proto`、`team.proto` 与 `common/v1/*.proto` 均满足上述形态，可直接沿用。

---

## 5. Server 实现（约束与示例）

### 5.1 绑定/校验

* **tenant 提取**：优先读 `RequestContext.tenant_id`，其次仅从 Metadata `tenant-id` 兜底（不接受任何 `x-powerx-*` 遗留租户头）。
  你的 `tenantIDFrom()` 已经实现这一落地逻辑。
* **分页映射**：`PageRequest(offset,page_size)` → `(page,size)` 的换算统一用工具函数（如 `pageFrom()`）。
* **错误回包**：Meta 中返回 `code/message/request_id`，与你的 `okMeta/badMeta` 一致（见 `member_handler.go`/`team_handler.go` 调用）。

### 5.2 调用 Service

* Server 仅调用同名用例，如 `MemberService.ListMembers/GetMember/...`；
* 严禁在 Server 内写 DB 或外部 IO 调用（与 HTTP 版一致）。

### 5.3 映射输出

* 领域对象 → PB：使用专用转换函数（如 `toPBMember()`），并保持 PRN/Ref 等字段预留。
* 分页总数写入 `PageResponse.total`，按需要计算 `pages`。

---

## 6. 拦截器（Auth / Tenant / Trace / Recovery）

* 统一在 `internal/transport/grpc/auth/middleware/auth_interceptor.go` 注册链：
  `Tracing → Auth/Tenant → Recovery → Logging/Audit（可选）`。
* Auth 成功后，将 `tenant_id`、`actor` 注入 `context.Context`，与 HTTP 中间件语义一致。
* Server 端实现保持**无状态**；租户与权限校验仍以 Service 为准。
  （你已存在 `auth_interceptor.go` 路径，保持即可。）

---

## 7. 错误映射（与 HTTP 等价）

| 应用错误                   | gRPC Code                                          |
| ---------------------- | -------------------------------------------------- |
| ErrInvalidParam / 参数缺失 | `codes.InvalidArgument`                            |
| ErrForbidden / 越权      | `codes.PermissionDenied`                           |
| ErrNotFound / 不存在      | `codes.NotFound`                                   |
| ErrConflict / 冲突       | `codes.AlreadyExists` 或 `codes.FailedPrecondition` |
| 其他未分类                  | `codes.Internal`                                   |

> 建议提供一个 `grpcerr.FromAppError(err)` 小工具集中处理映射；当前 `member_handler.go`/`team_handler.go` 以 “Meta + 200/400/500 语义”返回，你也可以逐步替换为标准 `status.Error` 形式以统一链路。

---

## 8. 流式（Server-Streaming）

* 与 SSE 事件顺序对齐：`start → data(token/partial) → final → end`；异常用 `error` 结束。
* 建议为长连接加入 heartbeat（超时时间与退避策略由客户端/SDK 控制）。
* 你的 `agent/stream_handler.go` 已是落地点，遵循以上语义即可。

---

## 9. 代码生成（Buf）

* 在 `api/grpc/contracts/` 下运行：

  ```
  buf lint
  buf breaking --against 'https://github.com/<org>/<repo>.git#branch=main'
  buf generate
  ```
* 生成物落在 `api/grpc/gen/go/**`，不要手改；SDK 封装在 `api/grpc/sdk/**`。

---

## 10. 验收要点（Checklist）

* [ ] **Server 零业务**：仅绑定/校验/调用 Service/错误映射（对齐 HTTP 版职能）。
* [ ] **多租户**：`tenant_id` 必带；缺失返回 400 语义或 `codes.InvalidArgument`。
* [ ] **分页一致**：PageRequest/PageResponse 与 HTTP 语义一致（`page/page_size/pages`）。
* [ ] **错误等价**：应用错误 → `codes.*`；或临时以 Meta 语义保持一致，逐步收敛至 `status.Error`。
* [ ] **拦截器链**：Tracing → Auth/Tenant → Recovery → Logging/Audit；服务端不重复做鉴权解析细节。
* [ ] **输出映射**：使用 `toPB*`，保留 `ResourceRef/PRN` 等字段空位。
* [ ] **生成流程**：`buf lint / breaking / generate` 通过；生成物与源码分离。
* [ ] **与 HTTP 等价**：同一用例在 HTTP 与 gRPC 的语义、错误、分页完全可对照。

---
