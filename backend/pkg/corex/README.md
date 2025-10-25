---

# CoreX Kernel（Library 模式）

> 目标：在 **Library 模式**下，为 PowerX 及其插件生态提供一个可嵌入的“内核基座”。
> 已有：`pkg/event_bus`（你的事件总线实现，本地/Redis/Fabricory 等）
> 本文档：规范 **组织架构（IAM）** + **RBAC** + **多租户/RLS** + **审计** +（规划中的）**Schema Registry / Data Broker / Admin Manifest** 的目录与最小接口。

---

## 目录结构

```
pkg/
└── corex/
    ├── ids/                # 统一ID（ULID/雪花）生成 & 解析
    ├── db/                 # gorm + migrate + WithTenant(ctx) 封装
    ├── rls/                # 多租户RLS辅助（SET app.tenant_id / 条件合并）
    ├── auth/               # JWT/中间件（复用你的现有实现或提供适配）
    ├── audit/              # 审计接口与默认实现（noop → 日志 → DB）
    ├── iam/                # 组织域（Tenant/User/Department/Membership）
    ├── rbac/               # 角色/权限/绑定 + 检查器（字段级 & 条件）
    ├── contracts/          # （规划）资源/视图契约模型与解析器
    ├── broker/             # （规划）Data Broker（v1只读 → v2写入）
    └── adminmanifest/      # （规划）Admin 菜单/路由清单拼装
```

> 事件平台（HTTP 发布入口、订阅投递等）依旧推荐放在你现有的 `pkg/event_bus` 下，以 `RegisterAPIRoutes` 暴露给外部工程挂载。CoreX 各域通过注入的 `EventBus` 发布领域事件。

---

## 一、快速开始（Quick Start）

### 1) 初始化内核（Library 嵌入）

```go
mux := http.NewServeMux()

// 1. 数据库（gorm） & 多租户会话
db := corexdb.OpenFromEnv() // 读取连接串/连接池配置
// 在你的HTTP中间件里：corexdb.WithTenant(ctx, db, tenantID) 注入租户

// 2. 事件总线（来自你已有 pkg/event_bus）
bus, _ := eventbus.NewEventBusFromEnv() // local / redis

// 3. 审计（可先使用 noop，或注入你的日志/DB实现）
aud := corexaudit.NewNoop()

// 4. IAM + RBAC 服务
iamSvc := corexiam.NewService(db, bus, aud)
rbacChk := corexrbac.NewChecker(db, aud)

// 5. 可选：一键注册路由（保持框架无关）
corexiam.RegisterAPIRoutes(mux, iamSvc)   // /iam/users /iam/departments /iam/roles...
corexrbac.RegisterAPIRoutes(mux, rbacChk) // /rbac/debug/policy 之类的调试接口
```

### 2) 在请求中注入租户（RLS）

```go
// 示例中间件（取自你现有JWT）：
func withTenant(next http.Handler) http.Handler {
  return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    tenantID := mustTenantIDFromJWT(r.Context())
    corexdb.WithTenant(r.Context(), corexdb.Global(), tenantID) // SET app.tenant_id
    next.ServeHTTP(w, r)
  })
}
```

---

## 二、基础包说明

### 1. `pkg/corex/ids/`

**职责**：提供统一的 ID 生成与解析，避免各域自行造轮子。
**最小接口**：

```go
id := corexids.New()         // "01HXY...ULID"
ok := corexids.IsValid(id)
```

---

### 2. `pkg/corex/db/`

**职责**：数据库通用封装与 migrate 入口；为 RLS 提供 `WithTenant`。
**最小接口**：

```go
db := corexdb.Open(cfg)                         // gorm.DB
corexdb.WithTenant(ctx, db, tenantID)          // SET app.tenant_id
corexdb.Migrate("corex", fs)                    // 可选：执行 corex schema 迁移
```

> 迁移建议：`db/migrations/corex/*`、`db/migrations/plugins/<id>/*` 分开管理。

---

### 3. `pkg/corex/rls/`

**职责**：行级安全（Row Level Security）规则拼接，供 Broker/仓储层合并。
**最小接口**：

```go
cond := corexrls.MustTenantFilter(ctx)     // e.g. tenant_id = $current_tenant
where := corexrls.And(cond, userFilter)    // 组合条件
```

---

### 4. `pkg/corex/audit/`

**职责**：统一审计钩子（API、Bus 发布/投递、RBAC 判定）。
**最小接口**：

```go
aud.LogAPI(ctx, "GET /iam/users", 200, latency)
aud.LogBusPublish(ctx, topic, subCount)
aud.LogBusDeliver(ctx, topic, pluginID, status, err)
aud.LogRBAC(ctx, subject, resource, action, allow)
```

> 默认 `Noop()`；可替换为写日志或落库实现。

---

### 5. `pkg/corex/iam/`

**职责**：组织与账号（Tenant/User/Department/Membership）域服务，发布领域事件。
**数据模型（Postgres，schema=`corex`）**：

```sql
create table corex.tenants(
  id text primary key, name text, created_at timestamptz default now()
);
create table corex.users(
  id text primary key, tenant_id text not null, account text unique,
  display_name text, active bool default true, created_at timestamptz default now()
);
create table corex.departments(
  id text primary key, tenant_id text not null, name text, parent_id text
);
create table corex.memberships( -- user ↔ dept
  user_id text, tenant_id text, dept_id text, primary key(user_id, dept_id)
);

-- RLS 示例（其他表同理）
alter table corex.users enable row level security;
create policy p_corex_users on corex.users using (tenant_id = current_setting('app.tenant_id')::text);
```

**最小服务接口**：

```go
type Service interface {
  CreateUser(ctx context.Context, in CreateUserDTO) (User, error)
  ListUsers(ctx context.Context, q ListQuery) (Page[User], error)
  CreateDepartment(ctx context.Context, in CreateDeptDTO) (Department, error)
  BindMembership(ctx context.Context, userID, deptID string) error
  // ...
}
```

**领域事件（通过 `pkg/event_bus` 发布）**：

* `iam.user.created` / `iam.user.updated` / `iam.user.disabled`
* `iam.department.created/updated`
* `iam.membership.bound/unbound`

> 事件上下文自动带上 `tenant_id / trace_id`（由你的 Event Bus 在 Publish 时从 ctx 注入）。

**路由注册（可选）**：

```go
func RegisterAPIRoutes(mux Mux, svc Service)
```

---

### 6. `pkg/corex/rbac/`

**职责**：角色与权限模型、绑定关系、判定引擎（支持字段级与条件）。
**数据模型**：

```sql
create table corex.roles(
  id text primary key, tenant_id text not null, name text
);
create table corex.permissions(
  id text primary key, tenant_id text not null,
  resource text not null, action text not null, fields jsonb, condition jsonb
);
create table corex.role_bindings(
  tenant_id text, role_id text, user_id text, primary key(tenant_id, role_id, user_id)
);
```

**判定引擎（最小接口）**：

```go
allow, fields, cond := corexrbac.Check(
  ctx, Subject{TenantID, UserID},
  Resource("ecom.order"), Action("read"),
  Attrs{"channel":"web"},
)
// allow=false → 403
// allow=true  → 在查询/序列化时：仅选择 fields，且把 cond 与 RLS 合并到 where
```

**路由注册（可选）**：

```go
func RegisterAPIRoutes(mux Mux, checker *Checker) // e.g. /rbac/debug/policy
```

---

## 三、与事件总线的衔接（已具备）

* **事件入口**：推荐在 `pkg/event_bus` 中提供 `RegisterAPIRoutes(mux, opts)` 暴露 `POST /_bus/publish`；CoreX 内核组件只需注入 `EventBus` 接口即可。
* **发布位置**：IAM/RBAC 在变更（如用户创建、角色绑定）时，向 `bus.Publish(topic, data)` 发领域事件。
* **订阅者**：插件侧实现 `/events` 接口接收；由宿主构建 `topic → subscribers` 路由索引并注入给总线入口。

---

## 四、（规划）扩展能力

### 1) `pkg/corex/contracts/`

* **作用**：解析 `plugins/<id>/contracts/index.yaml`、`resources/*.yaml`、`events/*.jsonschema`；输出可被 Broker & Admin 使用的资源/字段元数据。
* **接口**：

```go
reg := corexcontracts.NewRegistry()
reg.LoadFromDir("./plugins/*/contracts")
meta, _ := reg.GetResource("view.CalendarEvent:v1")
```

### 2) `pkg/corex/broker/`（v1 只读）

* **统一查询口**：`GET /v1/data/query?resource=...&fields=...&filter=...`
* **内核职责**：Scope 校验（RBAC）、字段裁剪、合并 RLS、执行计划（内核视图或插件 RPC）。

```go
rows, err := broker.Query(ctx, resource, fields, filter, sort, page)
```

### 3) `pkg/corex/adminmanifest/`

* **作用**：合并内核与插件清单（菜单/路由），按权限过滤，供前端 Admin 壳动态挂载。

```go
manifest := adminmanifest.Build(iam, rbac, plugins...)
```

---

## 五、落地顺序（建议）

**Phase 1（当前）**

* `db/`、`rls/`、`audit/`（基础）
* `iam/`（最小 CRUD + 事件）
* `rbac/`（最小判定 + 字段白名单）

**Phase 2**

* `contracts/`（解析注册）
* `broker/` v1（只读）
* `adminmanifest/`（菜单/路由清单）

**Phase 3**

* 审计落库、指标导出（Prometheus）
* RBAC 条件表达式增强、策略缓存
* Broker v2（写入/乐观锁/幂等）与小型联邦查询

---

## 六、权限与RLS的协同（关键约定）

* **每个请求**：JWT → `tenant_id/user_id/scopes` → 中间件注入 DB 会话 `SET app.tenant_id`。
* **查询类接口**（含 Broker）：

    1. RBAC 校验 `resource/action`
    2. **字段级裁剪**（字段白名单）
    3. **合并 RLS 条件**（`tenant_id` 与策略条件）
* **写入类接口**：RBAC 校验 `create/update/delete`；可选字段级写限制；落审计。

---

## 七、示例：创建用户并发事件

```go
// 由外部项目在 handler 中调用
func createUserHandler(w http.ResponseWriter, r *http.Request) {
  ctx := r.Context()
  in  := corexiam.CreateUserDTO{Account:"alice", DisplayName:"Alice"}
  u, err := iamSvc.CreateUser(ctx, in)
  if err != nil { /* write 400/500 */; return }
  // iamSvc 内部：写 corex.users → bus.Publish("iam.user.created", payload)
  writeJSON(w, u)
}
```

---

## 八、测试与开发约定

* **Unit Test**：各包提供基础单测（mock DB / stub EventBus / noop Audit）。
* **E2E**：建议保留一个最小外部工程示例，演示：

    * 初始化 CoreX（db + bus + iam + rbac）
    * 用户创建 → 事件发布 → 某“示例插件” `/events` 收到并落日志
    * RBAC 策略生效（字段裁剪 & RLS）

---

## 许可证 & 贡献

* 依照仓库根目录 `LICENSE`。
* 欢迎按此 README 的目录与接口提交最小 PR（先跑通 Phase 1）。

---

> 有了以上内核包，你就能：
>
> 1. 在 **Library 模式**内嵌 CoreX；
> 2. 通过 **Event Bus** 把 IAM/RBAC 等领域事件开放给插件；
> 3. 为后续的 **Data Broker / Admin Manifest** 直接接好“插口”。
