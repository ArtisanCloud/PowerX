---
name: crud-api-rest
description: PowerX REST 契约规则（资源命名、分页、错误、版本化）。
---

# PowerX CRUD API REST

## 步骤

1) 打开 `本文件内嵌规则`。
2) 按规则执行实现/校对。
3) 完成后按核对清单验收。

## 核对点

- 与 PowerX 当前代码结构、路径与命名一致。
- 仅在传输层/契约层做职责内改动，不跨层越界。

## 规则（内嵌）

### api_rest.yaml

````yaml
kind: ruleset
name: crud_api_rest
version: 1.0.0
owner: powerx
status: stable

meta:
  intent: >
    定义 PowerX 的 REST 契约基线：版本化路径、统一错误与分页信封、标准 CRUD 动词与资源命名、
    统一筛选/排序/搜索约定、SSE 事件名、速率限制与乐观并发，确保与 gRPC 在语义上可对照。
  references:
    - constitution.md
    - dev_crud_http_guides.md
    - dev_sts_guides.md

scope:
  applies_to:
    - "internal/transport/http/**/api.go"         # 路由注册文件需满足契约形态
    - "openapi/**/*.yaml"                          # 若你维护 OAS，亦按此规范校验（可选）
  api_versions:
    - "/api/v1/admin"
    - "/api/v1/open"
    - "/api/v1/web"                                # 仅当存在该前台接口时使用
    - "/api/v1/app"                               # 仅当存在在该前台APP接口时使用
  # 破坏性变更才允许升级 URL 版本（v1→v2）。:contentReference[oaicite:2]{index=2}

principles:
  - API 必须使用版本化前缀（/api/v1/*）；破坏性变更才升级版本。            # 路径/版本  :contentReference[oaicite:3]{index=3}
  - 统一错误信封：{ code, message, details?, request_id }；状态码集合固定。   # 错误结构/码  :contentReference[oaicite:4]{index=4}
  - 统一分页信封：pagination{ total,page,pageSize,pages }。                 # 分页字段     :contentReference[oaicite:5]{index=5}
  - 多租户上下文来自鉴权中间件/令牌，不允许以业务参数绕过。                 # 宪章-多租户  :contentReference[oaicite:6]{index=6}
  - 鉴权对齐 STS：HTTP 层若使用 JWT，应与 STS 的 KeyRing/issuer/kid 策略一致。 # STS 对齐   :contentReference[oaicite:7]{index=7}
  - SSE/WS 事件名统一（start/intent/plan/token/data/action/final/end/error/heartbeat）。 # 流式事件  :contentReference[oaicite:8]{index=8}
  - 与 gRPC 在错误/分页语义可一一对照（等价）。                             # gRPC 等价   :contentReference[oaicite:9]{index=9}

contracts:
  resource_naming:
    noun_style: kebab                   # e.g. /media/assets
    id_param: ":id"                    # /:id
    collection:
      verbs:
        create: { method: POST,   path: "" }
        list:   { method: GET,    path: "" }
      item:
        get:    { method: GET,    path: "/:id" }
        update: { method: PATCH,  path: "/:id" }
        delete: { method: DELETE, path: "/:id" }
    notes: "动词与路径需符合标准 CRUD 语义，保持与 handler 目录示例一致。"   # :contentReference[oaicite:10]{index=10}

  pagination:
    query_params:
      - { name: "page",      type: integer, default: 1,   min: 1 }
      - { name: "pageSize",  type: integer, default: 20,  min: 1, max: 200 }
      - { name: "sortBy",    type: string,  enum: ["createdAt","updatedAt","id"], optional: true }
      - { name: "sortOrder", type: string,  enum: ["asc","desc"], optional: true }
      - { name: "q",         type: string,  optional: true }   # 全文或关键字搜索
    response_shape:
      object: "ResponseList"
      fields:
        - "items: array<any>"
        - "pagination.total: integer"
        - "pagination.page: integer"
        - "pagination.pageSize: integer"
        - "pagination.pages: integer"
    must_align_with_http_guides: true   # 字段语义与指南一致  :contentReference[oaicite:11]{index=11}

  filtering:
    pattern: "filters[<field>]=<op>:<value>"
    ops:
      - "eq"     # 等于
      - "ne"     # 不等于
      - "in"     # 逗号分隔集合
      - "gte"    # ≥
      - "lte"    # ≤
      - "like"   # 模糊匹配
    examples:
      - "/media/assets?filters[status]=eq:1&filters[createdAt]=gte:2025-01-01"

  error_and_status:
    envelope: ["code","message","details?","request_id"]
    http_status_whitelist: [400,401,403,404,409,429,500]  # 统一状态集  :contentReference[oaicite:12]{index=12}
    mapping_notes: "与同名应用错误在 gRPC codes.* 上可对照。"                # :contentReference[oaicite:13]{index=13}

  auth_and_tenant:
    scheme: "Authorization: Bearer <JWT>"
    tenant_source: "从令牌解析；中间件注入到 ctx，不以 query/body 传递。"   # 宪章 & STS  :contentReference[oaicite:14]{index=14} :contentReference[oaicite:15]{index=15}
    sts_alignment: "验签使用与 STS 相同 KeyRing（包含 kid）与 issuer/aud 校验。" # :contentReference[oaicite:16]{index=16}

  concurrency_and_idempotency:
    etag:
      enabled_for: ["PATCH","DELETE"]
      headers: ["If-Match"]
      policy: "未匹配 ETag → 412（在实现层可折算为 409 语义）。"
    idempotency:
      enabled_for: ["POST"]
      header: "Idempotency-Key"
      ttl_seconds: 24*3600

  sse_and_ws:
    sse_content_type: "text/event-stream"
    events: ["start","intent","plan","token","data","action","final","end","error","heartbeat"]  # :contentReference[oaicite:17]{index=17}

  rate_limit:
    response_headers: ["X-RateLimit-Limit","X-RateLimit-Remaining","X-RateLimit-Reset"]
    on_exceed_status: 429

  content_negotiation:
    consume: ["application/json"]
    produce: ["application/json"]      # 流式除外（SSE）
    charset: "utf-8"

checks:
  versioned_paths:
    - id: api.version.prefix
      level: error
      when: { glob: "internal/transport/http/**/api.go" }
      assert:
        - must_prefix_route_one_of: ["/api/v1/admin","/api/v1/open","/api/v1/web"]   # :contentReference[oaicite:18]{index=18}

  crud_routes_shape:
    - id: routes.crud.shape
      level: error
      when: { glob: "internal/transport/http/**/api.go" }
      assert:
        - must_register_methods:
            - "POST \"\""
            - "GET \"\""
            - "GET \"/:id\""
            - "PATCH \"/:id\""
            - "DELETE \"/:id\""              # 形态对齐示例路由  :contentReference[oaicite:19]{index=19}

  envelope_and_codes:
    - id: response.error.schema
      level: error
      when: { glob: "internal/transport/http/**/**_handler.go" }
      assert:
        - must_use_error_bridge: ["RespondErrorFrom","ResponseSuccess"]  # 统一错误桥接
        - http_status_in: [400,401,403,404,409,429,500]         # 统一状态集    :contentReference[oaicite:21]{index=21}

  pagination_contract:
    - id: pagination.contract
      level: error
      when: { glob: "internal/transport/http/**/**_handler.go" }
      assert:
        - must_bind_dto: ["PaginationRequest"]
        - must_return: ["ResponseList","PaginationResponse"]    # 统一分页信封  :contentReference[oaicite:22]{index=22}

  sse_contract:
    - id: sse.events
      level: warn
      when: { contains: "WriteToSSE(" }
      assert:
        - must_use_events: ["start","intent","plan","token","data","action","final","end","error","heartbeat"]  # :contentReference[oaicite:23]{index=23}

  auth_alignment:
    - id: auth.sts.aligned
      level: warn
      when: { glob: "internal/transport/http/**/**_handler.go" }
      assert:
        - must_verify_jwt_with_keyring: true      # 与 STS KeyRing 对齐（issuer/aud/kid）  :contentReference[oaicite:24]{index=24}

acceptance:
  checklist:
    - "[ ] 路由使用 /api/v1/{admin|open|web|app} 前缀；破坏性变更才升级版本"           # :contentReference[oaicite:25]{index=25}
    - "[ ] CRUD 形态：POST/GET/GET/:id/PATCH/:id/DELETE/:id 全量存在"             # :contentReference[oaicite:26]{index=26}
    - "[ ] 错误结构统一，状态码限定在 {400,401,403,404,409,429,500}"              # :contentReference[oaicite:27]{index=27}
    - "[ ] 分页响应包含 total/page/pageSize/pages，列表封装在 ResponseList"       # :contentReference[oaicite:28]{index=28}
    - "[ ] SSE/WS（若有）事件名与规范一致"                                         # :contentReference[oaicite:29]{index=29}
    - "[ ] 鉴权对齐 STS：JWT 验签与 KeyRing/issuer/aud/kid 一致"                  # :contentReference[oaicite:30]{index=30}
    - "[ ] （可选）支持 Idempotency-Key 与 If-Match(Etag) 的幂等与并发控制"
    - "[ ] 与 gRPC 的分页/错误语义可对照（等价）"                                  # :contentReference[oaicite:31]{index=31}

templates:
  # OpenAPI 片段（如你维护 OAS）
  openapi_snippet: |
    paths:
      /api/v1/admin/media/assets:
        get:
          summary: List assets
          parameters:
            - in: query; name: page; schema: { type: integer, minimum: 1, default: 1 }
            - in: query; name: pageSize; schema: { type: integer, minimum: 1, maximum: 200, default: 20 }
            - in: query; name: sortBy; schema: { type: string, enum: [createdAt, updatedAt, id] }
            - in: query; name: sortOrder; schema: { type: string, enum: [asc, desc] }
            - in: query; name: q; schema: { type: string }
          responses:
            "200":
              description: OK
              content:
                application/json:
                  schema:
                    $ref: "#/components/schemas/ResponseList"
            "400": { $ref: "#/components/responses/BadRequest" }
            "401": { $ref: "#/components/responses/Unauthorized" }
            "403": { $ref: "#/components/responses/Forbidden" }
            "404": { $ref: "#/components/responses/NotFound" }
            "409": { $ref: "#/components/responses/Conflict" }
            "429": { $ref: "#/components/responses/TooManyRequests" }
            "500": { $ref: "#/components/responses/InternalError" }

  router_go: |
    func Register{{Domain}}Routes(rg *gin.RouterGroup, deps *shared.Deps) {
      h := New{{Entity}}Handler(deps.{{Entity}}Service)
      g := rg.Group("/{{domain}}/{{resource}}")
      {
        g.POST("", h.Create)
        g.GET("", h.List)
        g.GET("/:id", h.Get)
        g.PATCH("/:id", h.Update)
        g.DELETE("/:id", h.Delete)
      }
    }
````
