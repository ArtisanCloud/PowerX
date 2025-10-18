# PowerX gRPC 设计与使用指南（api/grpc/readme.md）

> 本文是 **`api/grpc/`** 目录的总览与使用说明。面向：PowerX 核心服务开发者、插件开发者、测试与运维同学。
> 目标：**契约集中、实现解耦、对外一致、可本地调试**。

---

## 目录结构（放在 `api/grpc/`）

```
api/grpc/
├─ contracts/                 # 契约中心：*.proto 与契约文档
│  ├─ common/                 # 通用类型：RequestContext、ResourceRef(PRN)、Money、Pagination...
│  ├─ iam/                    # 身份/授权：identity.proto、authorization.proto...
│  ├─ org/                    # 组织域：tenant.proto、member.proto、department.proto、team.proto...
│  ├─ plugin/                 # 插件域：plugin.proto、registry.proto、plugin_config.proto
│  ├─ agent/                  # Agent 域：agent.proto、tool.proto、workflow.proto
│  ├─ system/                 # 系统域：config.proto、notification.proto、audit.proto、metrics.proto、health.proto
│  ├─ data/                   # 数据域：event.proto、broker.proto（查询联邦/Resolver SPI）
│  ├─ ext/                    # 扩展契约（第三方/垂直域）
│  ├─ versioning/             # 契约版本与兼容性规则（ADR/MD）
│  ├─ buf.yaml                # Buf 配置（lint/依赖）
│  ├─ buf.gen.yaml            # 代码生成矩阵（Go/TS/Java…）
│  └─ README.md               # 契约命名规范/提交流程
│
├─ gen/                       # 生成物：由 buf/protoc 生成（请勿手改）
│  ├─ go/
│  ├─ ts/
│  ├─ java/
│  └─ openapi/
│
├─ sdks/                      # 面向插件的友好 SDK 封装（便于安装与调用）
│  ├─ go/powerx-grpc-go/      # Go module（封装拨号、重试、鉴权、PRN、分页/流）
│  └─ ts/packages/            # NPM 包（@powerx/grpc）
│
├─ gateway/                   # 对外统一入口（Envoy / gRPC-Gateway）
│  ├─ envoy/
│  ├─ grpc-gateway/
│  └─ config/
│
├─ mock/                      # 本地/CI 联调用 Mock Server
│  ├─ server/
│  ├─ scenarios/
│  └─ data/
│
├─ discovery/                 # 服务发现与端点目录
│  ├─ services.yaml           # 逻辑服务名 → 端点映射（dev/stg/prod）
│  └─ public-endpoints.yaml   # 对插件公开的网关域名清单
│
├─ docs/                      # 面向插件开发者的文档
│  ├─ quickstart-plugin.md
│  ├─ service-catalog.md
│  ├─ auth.md
│  ├─ prn.md
│  └─ debug.md
│
├─ tools/                     # 工程化工具（脚本、lint、breaking-check）
│  ├─ protogen/
│  ├─ lint/
│  └─ breaking-check/
│
├─ examples/                  # 端到端示例（插件脚手架/跨域查询）
│  ├─ plugin-skeleton/
│  └─ ecom-scrm-bridge/
│
└─ test/
   └─ conformance/            # 多语言 SDK 互操作一致性测试
```

---

## 设计约定与关键概念

### 1) 领域命名：`domain.entity`

* 与 DDD 有界上下文一致：`iam.member`、`org.department`、`ecom.order`…
* 事件主题与 gRPC 包名保持一致前缀（便于检索与治理）。

### 2) 资源标识（PRN）与引用（ResourceRef）

* **三层标识**：

    * `id`（稳定主键，ULID/Snowflake，机器友好，不变）
    * `human_key`（人类可读业务号，如 `ORD-2025-08-000012`）
    * `prn`（PowerX 统一资源名，跨系统唯一引用）
* PRN 规范（示例）：
  `prn:powerx:prod:t-42:ecom.order:01J7E9RC2F8Y2JJH6V8B7QZ4MN`
* **Proto 建议（`contracts/common/resource.proto`）**：

  ```protobuf
  message ResourceRef {
    int64  tenant_id = 1;
    string domain    = 2;   // "ecom" / "iam" / ...
    string entity    = 3;   // "order" / "member" / ...
    string id        = 4;   // ULID/Snowflake
    string human_key = 5;   // 可选：ORD-2025-08-000012
    string prn       = 6;   // prn:powerx:...
    map<string,string> labels = 7;
  }
  ```

### 3) 请求上下文（RequestContext）

* **所有 RPC 必带**：`tenant_id`、`member_id`、`access_token/scopes`、`request_id`…
* 由网关/客户端中间件自动注入，服务端拦截器统一校验（RBAC/ABAC + RLS）。

### 4) 数据共享与一致性

* **权威 API**：强一致读/必要写，归属域负责。
* **事件总线**：`*.created/updated/deleted` 最终一致，Outbox + Relay 保证送达。
* **DataBroker**：跨域组合查询（Resolver SPI），可缓存，可 explain 血缘。

### 5) 版本管理（SemVer + breaking-check）

* `.proto` 放在 `contracts/`，使用 Buf 做 **lint** 与 **breaking-change** 检查。
* `MAJOR` 允许破坏性变更（双发期）；`MINOR` 后向兼容新增字段；`PATCH` 修复。

---

## 生成代码（Buf / Protoc）

> 在 `api/grpc/contracts/` 目录执行：

```bash
# Lint & 破坏性变更检查（与 main 分支或上一个 tag 比对）
buf lint
buf breaking --against 'https://github.com/<org>/<repo>.git#branch=main'

# 代码生成（根据 buf.gen.yaml 输出到 api/grpc/gen/*）
buf generate
```

# Buf 配置要点

- `buf.gen.yaml` 的 `managed.go_package_prefix` 使用 **overrides** 指定 `corex/event_fabric/v1` 的 Go 包前缀，确保生成代码落在 `api/grpc/gen/go/corex/event_fabric/v1`，与目录结构和 import 规范一致。
- 旧有契约若暂不符合 lint 规则（例如 `powerx/capability/registry/v1/registry.proto` 中的请求命名），通过 `buf.yaml` 的 `ignore_only` 精确豁免 `RPC_REQUEST_STANDARD_NAME`。新增契约仍必须遵守规范，避免扩大忽略范围。

典型输出：

* Go：`api/grpc/gen/go/powerx/{iam,org,plugin,agent,system,data}/v1/...`
* TS：`api/grpc/gen/ts/...`
* OpenAPI（可选）：`api/grpc/gen/openapi/...`

> **不要手改 `gen/` 目录**；如需便捷 API，请在 `sdks/` 中封装。

---

## SDK（面向插件）

### Go（`sdks/go/powerx-grpc-go`）

* 功能：自动拨号（读取 `discovery/services.yaml`）、注入 `RequestContext`、重试/超时/熔断、PRN 工具、分页/流式 Helper。
* 使用示例：

  ```go
  c, _ := powerx.NewClient(powerx.Config{
      Gateway: os.Getenv("POWERX_GRPC_GATEWAY"), // 如：gateway.powerx.local:443
      Token:   os.Getenv("POWERX_CAP_TOKEN"),    // Capability Token (JWT)
      Tenant:  42,
  })
  resp, err := c.IAM.Identity.Authenticate(ctx, &iam.AuthenticateRequest{ /* ... */ })
  ```

### TypeScript（`sdks/ts/packages/@powerx/grpc`）

* 支持 Node 与 grpc-web（浏览器/前端），中间件自动附带 Token 与 `tenant_id`。
* 使用示例（Node）：

  ```ts
  const client = createPowerXClient({
    gateway: process.env.POWERX_GRPC_GATEWAY!,
    token: process.env.POWERX_CAP_TOKEN!,
    tenant: 42,
  })
  const me = await client.iam.identity.getCurrentUser({})
  ```

---

## 网关与服务发现

### 统一网关（`gateway/`）

* Envoy / gRPC-Gateway：统一 TLS/mTLS、限流、熔断、重试、路由、反射（便于 grpcurl/grpcui）。
* 插件只需连一个网关地址：`POWERX_GRPC_GATEWAY=...`

### 发现配置（`discovery/services.yaml`）

* 逻辑服务名 → 端点映射（dev/stg/prod），SDK 自动读取；也可用环境变量覆盖：

    * `POWERX_GRPC_GATEWAY`、`POWERX_ENV`、`POWERX_TENANT_ID`

---

## 鉴权与租户上下文（插件安装后可调用）

1. 插件在安装时向宿主申请 **Data Scopes**（写在 `plugin.yaml`）
2. 平台签发 **Capability Token (JWT)**，包含 `aud=powerx`、`tenant_id` 以及 `scopes=[...]`
3. SDK 将 Token 与 `tenant_id` 注入 `RequestContext`，网关与后端共同校验
4. 可选 mTLS（开发环境提供自签发证书）

详见：`docs/auth.md`

---

## 本地与 CI 调试

### Mock Server（`mock/server/`）

* 一键启动：提供模拟的 IAM/Org/Ecom… 响应，支持场景脚本（`mock/scenarios/`）
* 适合插件在未接入真实后端前的联调与契约测试（CI 可复用）

### 调试工具（`docs/debug.md`）

* 反射开启后，列出服务：

  ```bash
  grpcurl -plaintext $POWERX_GRPC_GATEWAY list
  ```
* 调用示例（带 Token）：

  ```bash
  grpcurl -H "Authorization: Bearer $POWERX_CAP_TOKEN" \
    -d '{"ctx":{"tenantId":42},"username":"mike","password":"***"}' \
    $POWERX_GRPC_GATEWAY powerx.iam.v1.IdentityService.Authenticate
  ```
* 图形界面：`grpcui` 连接网关进行交互式调试。

---

## 事件与 DataBroker（跨域数据）

### 事件（`contracts/data/event.proto`）

* 主题：`{domain}.{entity}.{event}`，如 `ecom.order.created`
* 载荷包含 `ResourceRef subject`、`version`、`sequence`、`occurred_at`、`tenant_id`、`trace_id`
* 生产者采用 **Outbox + Relay**；消费者实现 **幂等** 与 **DLQ/重试**

### DataBroker（`contracts/data/broker.proto`）

* 用于跨域组合查询与血缘追踪（Resolver SPI）：

    * `Resolve(query, vars, bypass_cache)`
    * `Explain(query)` 返回血缘与执行计划
* 典型用法：SCRM 查询「客户 90 天订单总额」由电商域 Resolver 提供。

---

## Handler 返回规范

PowerX gRPC CRUD 接口需要与 HTTP 层的 `pkg/dto.BaseResponse` 保持等价语义，统一使用 `common.v1.ResponseMeta` 封装成功与失败。

- **统一信封**：`ResponseMeta{code,message,timestamp,request_id}` 必填；成功固定 `code=200,message="success"`，失败对齐 HTTP 的 4xx/5xx。
- **数据载荷**：领域结果置于 `data`（如 `FooData`、`ListFooData`）；无内容时可省略或仅返回 `bool ok`。
- **错误详情**：`ErrorExtra` 承载原始错误字符串与结构化 `details`（例如校验问题列表）。
- **请求上下文**：从 `RequestContext` 或 metadata 提取 `tenant_id`、`request_id`；缺少租户直接返回 400。
- **gRPC status 使用边界**：仅当无法构造业务响应（序列化失败、上下游不可用等）时返回 `status.Errorf` 的非 OK 状态。

示例模板：

```go
func (s *FooServer) GetFoo(ctx context.Context, req *foov1.GetFooRequest) (*foov1.GetFooResponse, error) {
    tid := tenantIDFrom(ctx, req.GetCtx())
    if tid == 0 {
        return &foov1.GetFooResponse{
            Meta: badMeta(ctx, http.StatusBadRequest, "tenant_id required"),
        }, nil
    }

    domain, err := s.fooSvc.GetFoo(ctx, uint64(tid), req.GetId())
    if err != nil {
        if errors.Is(err, service.ErrNotFound) {
            return &foov1.GetFooResponse{
                Meta: badMeta(ctx, http.StatusNotFound, "foo not found"),
            }, nil
        }
        return nil, status.Errorf(codes.Internal, "get foo: %v", err)
    }

    return &foov1.GetFooResponse{
        Meta: okMeta(ctx),
        Data: &foov1.GetFooData{Foo: toPBFoo(domain)},
    }, nil
}
```

建议将 `okMeta/badMeta/errorExtra` 等工具函数放入 `internal/transport/grpc/common/meta.go`，方便各 Handler 复用。

---

## 约束与最佳实践（务必遵守）

* **所有 RPC 均需携带 `RequestContext`，且必须含 `tenant_id`**。
* **禁止跨 Schema 物理外键**；跨域仅通过 `ResourceRef` 引用。
* **PRN 只读、可派生，不含 PII**；对外展示用 `human_key`。
* **变更发布遵循 SemVer**，提交前必须通过 `buf lint` 与 `buf breaking`。
* **事件发布必须走 Outbox**；消费者实现幂等（以 `subject.prn` + `version` 为键）。
* **对插件暴露仅通过网关**（方便统一观测、熔断、限流与审计）。
* **审计必做**：拦截器记录 who/what/why/when（PRN + request\_id + trace\_id）。

---

## 常用命令速查

```bash
# 进入契约目录
cd api/grpc/contracts

# Lint
buf lint

# 破坏性变更检查（与 main 分支）
buf breaking --against 'https://github.com/<org>/<repo>.git#branch=main'

# 生成多语言代码
buf generate

# 列出网关可用服务（本地/测试环境）
grpcurl -plaintext $POWERX_GRPC_GATEWAY list

# 通过网关调用一个方法
grpcurl -H "Authorization: Bearer $POWERX_CAP_TOKEN" \
  -d '{"ctx":{"tenantId":42}}' \
  $POWERX_GRPC_GATEWAY powerx.system.v1.HealthService.Check
```

---

## 启动清单（Checklist）

* [ ] 在 `contracts/common/` 定义 `RequestContext`、`ResourceRef(PRN)`、`Money/Pagination/TimeRange`
* [ ] 补齐 `buf.yaml / buf.gen.yaml`（Go/TS/OpenAPI）
* [ ] `gateway/` 配置（反射 + TLS/mTLS + 超时/限流/熔断）
* [ ] `sdks/` 发布管道（Go module & NPM）
* [ ] `discovery/services.yaml`（dev/stg/prod 端点）
* [ ] `mock/server` 可运行，含基础场景
* [ ] `docs/` 写全：`quickstart-plugin.md`、`auth.md`、`debug.md`、`service-catalog.md`
* [ ] CI 集成：`buf lint`、`buf breaking`、SDK 构建与发布、conformance 测试
* [ ] Observability：网关与服务端开启日志/指标/Trace，审计与血缘记录 PRN

---

**下一步**
你可以直接按此结构创建 `api/grpc/`，我可以继续给你：

* `buf.yaml / buf.gen.yaml` 最小可用模板
* `common/resource.proto` 与 `common/context.proto` 样例
* Go/TS SDK 的最小拨号与拦截器代码片段
* 一个 `mock/server` 的可运行脚手架（便于插件即时联调）
