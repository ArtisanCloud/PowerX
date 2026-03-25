---
name: crud-http
description: PowerX CRUD HTTP 开发规范（管理端路由、绑定、错误桥接、多租户）。
---

# PowerX CRUD HTTP

## 步骤

1) 打开 `本文件内嵌规则`。
2) 按规则执行实现/校对。
3) 完成后按核对清单验收。

## 核对点

- 与 PowerX 当前代码结构、路径与命名一致。
- 仅在传输层/契约层做职责内改动，不跨层越界。

## 规则（内嵌）

### dev_crud_http_guides.md

````markdown
# PowerX 开发规范入口（Dev Guides）

> 本文档是 PowerX 的统一开发规范入口。
>
> - 人类可阅读：开发与评审统一看这里。
> - 工具可扩展：未来可被 `/plan` 与 `/tasks` 解析成可验证规则。

---

## 1. 目的与范围（Scope）

本规范适用于所有 PowerX 核心与插件模块中涉及 **CRUD 能力** 的后端开发，  
包括但不限于以下层次：

- 迁移（Migration）
- GORM 模型（Model）
- Repository（仓库层，含 BaseRepository）
- Service（业务服务层）
- DTO（入参/出参）
- Handler（HTTP 层）
- 依赖注入（DI）
- 路由（Routes）
- 契约 / 集成测试（Tests）

---

## 2. 全局策略（Policies）

- **数据库**：统一使用 **GORM 抽象**，默认 **Postgres**。  
  不允许写死 MySQL 方言或独立 SQL 文件；索引与唯一约束以 GORM tag 或迁移注册为准。
- **HTTP 路由**：统一前缀 `/api/v1/admin/**`（可配置）。
- **多租户**：所有查询与写入都必须携带 `tenant_id`。
- **审计**：所有写操作（Create/Update/Delete/Presign）必须记录审计日志。
- **安全**：预签名链接仅允许受管路径、受限方法（GET/PUT）与 MIME 白名单。
- **依赖注入**：所有仓库与服务实例必须由 `internal/app/shared/deps.go` 统一注册。

---

## 3. 层次结构（Layer Overview）

```plaintext
Model → Repository → Service → Handler → Router
                  ↘ DI(shared) ↙
```

---

## 4. Migration（迁移）

### 约定

- **统一入口**：`cmd/database/migrate.go`
  由该入口统筹调用各模块的迁移函数。
- **两种迁移模式**

  1. **独立 Server 模块（如 Agent）**：在 `cmd/database/migrate.go` 内实现 `MigrateAgentModels(db *gorm.DB) error`，仅使用 `db.AutoMigrate(...)` 注册模型。
  2. **CoreX 内核模块（如 MediaX）**：在 `pkg/corex/db/migration.go` 内实现 `MigrateCoreModels(db *gorm.DB) error`，由其中**直接调用 `AutoMigrate`** 将核心模型（含 MediaX）纳入迁移。
- **不交付任何 `.sql` 文件**，迁移中禁止手写 `db.Exec(...)`。
- **索引/唯一约束**：一律在 **GORM 模型结构体标签**中声明（含唯一索引、GIN 索引、部分索引等）。
- **运行期校验（可选）**：如需校验可用 `db.Migrator().HasIndex()` / `HasConstraint()`，但**不得因此写原生 SQL**。
- **错误处理**：所有迁移函数必须返回 `error`，并在统一入口串联调用。

---

### 示例

#### A. 独立 Server 模块（Agent）

> 在 `cmd/database/migrate.go` 内直接定义并调用：

```go
// cmd/database/migrate.go
package main

import (
    "log"
    "gorm.io/gorm"

    dbmodel "github.com/your/module/internal/server/agent/persistence/model"
)

func MigrateAgentModels(db *gorm.DB) error {
    if err := db.AutoMigrate(
        &dbmodel.Agent{},
        &dbmodel.AgentSetting{},
    ); err != nil {
        return err
    }
    // 可选：运行期校验（不写 SQL）
    if ok := db.Migrator().HasIndex(&dbmodel.Agent{}, "agent_uniq_tenant"); !ok {
        log.Println("warn: agent_uniq_tenant not created")
    }
    return nil
}

func main() {
    // init db ...
    // 串联调用（示例）
    if err := MigrateCoreModels(db); err != nil { log.Fatal(err) }
    if err := MigrateAgentModels(db); err != nil { log.Fatal(err) }
}
```

#### B. CoreX 内核模块（含 MediaX）

> 在 `pkg/corex/db/migration.go` 中集中维护 CoreX 的模型清单（例如 MediaX）并 **只用 `AutoMigrate`**：

```go
// pkg/corex/db/migration.go
package corexdb

import (
    "gorm.io/gorm"

    mediamodel "github.com/your/module/pkg/corex/db/persistence/model/media"
    // 其他 CoreX 内核模型...
)

func MigrateCoreModels(db *gorm.DB) error {
    return db.AutoMigrate(
        &mediamodel.MediaAsset{},
        // &mediamodel.MediaSet{}, // 如有
        // 其他 CoreX 内核模型...
    )
}
```

> 说明：
>
> - **MediaX 等内核能力**作为 CoreX 的一部分，统一纳入 `MigrateCoreModels`；
> - 索引/约束通过模型标签自动创建（如 `unique`、`using:gin`、`where:deleted_at IS NULL` 等）；
> - 不再需要也不建议为 MediaX 再单独写 `MigrateMediaModels`。

### 示例

```go
func MigrateAgentModels(db *gorm.DB) error {
    db.AutoMigrate(
        &dbmodel.Agent{},
        &dbmodel.AgentSetting{},
    )
    if ok := db.Migrator().HasIndex(&dbmodel.Agent{}, "agent_uniq_tenant"); !ok {
        log.Println("warn: agent_uniq_tenant not created")
    }
    return nil
}
```

### 验收要点

- [ ] 模块定义独立的 `Migrate<Module>Models(db *gorm.DB)` 函数
- [ ] 模型通过 `db.AutoMigrate()` 注册，无需 .sql 文件
- [ ] 索引与唯一约束在 GORM 模型 tag 中声明
- [ ] （可选）运行期校验索引 `db.Migrator().HasIndex()`
- [ ] 迁移函数返回 `error` 并在顶层入口统一调用

---

## 5. Model（GORM 模型）

### 约定

- 模型统一基于 **PowerX Base Model**：

  - `PowerModel`：自增整型主键；
  - `PowerUUIDModel`：双主键（UUID + 自增 ID），在 `BeforeCreate()` 自动生成 UUID。
- 必须显式声明：

  - `CreatedAt` / `UpdatedAt` / `DeletedAt`；
  - 表名函数 `TableName()` 与 `GetTableName()`；
- 多租户字段：`TenantID uint64`；
- Schema 默认由 `PowerXSchema` 控制；
- 表名常量集中在 `tables.go`；
- JSON 字段使用 `datatypes.JSON`；
- 唯一/复合索引通过 GORM tag 定义；
- 状态字段统一命名 `Status`，默认值 1；
- 模型文件放在 `pkg/corex/db/persistence/model/<domain>/`。

### 示例

```go
type User struct {
    model.PowerUUIDModel
    Email       string         `gorm:"column:email;uniqueIndex:uk_user_email"`
    DisplayName string         `gorm:"type:varchar(128)"`
    Meta        datatypes.JSON `gorm:"type:jsonb;default:'{}'"`
    DeletedAt   gorm.DeletedAt `gorm:"index"`
}
func (u *User) TableName() string {
    return model.PowerXSchema + "." + model.TableIAMUser
}
```

### 验收要点

- [ ] 模型嵌入 `PowerModel` 或 `PowerUUIDModel`
- [ ] 含软删字段 `DeletedAt` 并建索引
- [ ] 含租户字段 `TenantID`
- [ ] JSONB 字段正确定义
- [ ] 表名函数返回 `${schema}.${table}`
- [ ] 索引通过 GORM tag 定义
- [ ] 状态字段默认 1
- [ ] 模型对应迁移函数已注册

---

## 6. Repository（仓库层）

### 约定

- 所有仓库组合 `BaseRepository[T]`；
- 所有方法均接收 `context.Context`；
- 写操作返回 `error`；
- 禁止直接操作 DB 或外部 IO；
- 唯一冲突用 `IsUniqueViolation()`；
- 在 `deps.go` 统一注册。

### 示例

```go
type UserRepository struct {
    *repository.BaseRepository[dbm.User]
    db *gorm.DB
}
func NewUserRepository(db *gorm.DB) *UserRepository {
    return &UserRepository{
        BaseRepository: repository.NewBaseRepository[dbm.User](db),
        db:             db,
    }
}
```

### 验收要点

- [ ] 组合 `BaseRepository[T]`
- [ ] 构造函数 `New<ModelName>Repository` 存在
- [ ] 方法首参为 `context.Context`
- [ ] 写操作返回 `error`
- [ ] 在 `deps.go` 注册
- [ ] 不含外部 IO 操作

---

## 7. Service（业务服务层）

### 设计原则

Service 层负责 **业务用例编排、鉴权、事务、审计**。
所有数据库操作必须通过 Repository 完成，不得直接调用 GORM 原语或访问外部 IO。

### 约定

- 命名：`<ModelName>Service`；
- 构造函数：`New<ModelName>Service(db *gorm.DB, …)`；
- 首参 `context.Context`；
- 组合 `*BaseService`；
- 统一在 `deps.go` 注册；
- 写操作必须事务化并记录审计；
- 业务错误集中定义于 `internal/service/errors.go`；
- 更新走白名单更新；
- 删除默认软删，硬删需显式授权；
- 错误包装统一。

### 验收要点

- [ ] 组合 `*BaseService`
- [ ] 构造函数存在
- [ ] 方法首参 `context.Context`
- [ ] 不直接访问 DB
- [ ] 写操作包裹事务与审计
- [ ] 删除默认软删，Force 硬删需权限
- [ ] 错误集中定义
- [ ] 已在 `deps.go` 注册

---

## 8. DTO & Validation（数据传输与校验）

### 统一响应与分页

- 成功/失败结构：`ResponseSuccess`、`ResponseError`、`ResponseList`；
- 分页结构：`PaginationRequest`, `PaginationResponse`。

### 参数绑定与校验

- 使用 `ValidateRequestWithContext(c, req)`；
- DTO 字段带 `validate` 标签；
- 校验失败返回统一结构。

### 错误桥接

- 统一错误结构：`AppError{HTTPCode, Message, Details}`；
- 桥接方法：`RespondErrorFrom(c, err)`，成功时直接使用 `ResponseSuccess/ResponseList`，**不得再使用 `MustOK`**。

### SSE / WS

- 事件名统一：`start/intent/plan/token/data/action/final/end/error/heartbeat`；
- SSE 写入：`WriteToSSE(c, flowID, execID, sr, heartbeat)`；
- WS 信封：`WSEnvelope{Type, Data, Timestamp}`。

### 验收要点

- [ ] DTO 独立定义，不复用模型；
- [ ] 参数绑定使用统一函数；
- [ ] 响应结构统一；
- [ ] 错误使用 AppError；
- [ ] 流式接口事件名统一；

---

## 9. Handler & Routes（HTTP）

### 目录结构

```bash
internal/transport/http/
  admin/<domain>/
    api.go                 # 路由注册
    <feature>_handler.go   # 纯 Handler：绑定/校验/DTO映射/调用Service/统一回包
```

### 路由示例

```go
func RegisterMediaRoutes(rg *gin.RouterGroup, deps *shared.Deps) {
    h := NewMediaHandler(deps.MediaService)
    g := rg.Group("/media/assets")
    {
        g.POST("", h.Create)
        g.GET("", h.List)
        g.GET("/:id", h.Get)
        g.PATCH("/:id", h.Update)
        g.DELETE("/:id", h.Delete)
    }
}
```

### 验收要点

- [ ] `api.go` 只注册路由；
- [ ] Handler 只做绑定/校验/调用 Service；
- [ ] 使用统一回包函数；
- [ ] 前缀 `/api/v1/admin`；
- [ ] 动词与路径符合 REST 语义；
- [ ] 契约测试覆盖 CRUD。

---

## 10. Dependency Injection（依赖注入）

### 约定

- 所有依赖集中在 `internal/app/shared/deps.go`；
- 统一构造函数 `NewDeps(db, opts)`；
- 所有服务在 `Deps` 结构体中注册；
- Handler 层通过 `*shared.Deps` 访问；
- 不在模块中重复创建连接；
- Audit 与 Auth 统一实例。

### 验收要点

- [ ] 所有依赖集中初始化；
- [ ] Handler 统一接收 `*shared.Deps`；
- [ ] Service 不自行创建连接；
- [ ] Audit 回调注册成功；
- [ ] 新模块扩展遵循 Deps + Options 模式。

---

## 11. API 契约（REST）

### 路径与版本

- 管理后台：`/api/v1/admin`
- 开放接口：`/api/v1/open`
- Web 前台：`/api/v1/web`
- URL 版本化策略，破坏性变更才升级。

### 错误与分页

- 统一错误结构：`{ code, message, details?, request_id }`；
- 状态码：400/401/403/404/409/429/500；
- 分页响应带 `pagination{total,page,pageSize,pages}`。

### 验收要点

- [ ] API 路径与版本规范；
- [ ] 错误结构统一；
- [ ] 分页字段完整；
- [ ] 幂等与权限策略明确；
- [ ] SSE/WS 事件一致。

---

## 12. 测试（Contracts & Integration）

### 约定

- 契约测试覆盖鉴权/分页/筛选/软删；
- 集成测试覆盖完整业务流；
- 使用 `httptest`；
- 模拟外部依赖可使用 mock。

### 验收要点

- [ ] 契约测试完整；
- [ ] 集成测试覆盖 CRUD；
- [ ] 测试使用 httptest；
- [ ] mock 外部依赖。

---

## 13. 附录：文件结构参考

```
pkg/corex/db/persistence/model/<feature>/
pkg/corex/db/persistence/repository/<feature>/
internal/service/<feature>/
internal/transport/http/admin/<feature>/
internal/app/shared/deps.go
specs/<domain>/<feature>/    # 功能规格与规划 (spec.md / plan.md / tasks.md)
```
````
