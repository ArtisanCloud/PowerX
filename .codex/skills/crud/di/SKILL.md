---
name: crud-di
description: PowerX CRUD 依赖注入规则（Deps 单入口、构造注入、跨传输复用）。
---

# PowerX CRUD DI

## 步骤

1) 打开 `本文件内嵌规则`。
2) 按规则执行实现/校对。
3) 完成后按核对清单验收。

## 核对点

- 与 PowerX 当前代码结构、路径与命名一致。
- 仅在传输层/契约层做职责内改动，不跨层越界。

## 规则（内嵌）

### di.yaml

````yaml
kind: ruleset
name: crud_di
version: 1.0.0
owner: powerx
status: stable

meta:
  intent: >
    统一依赖注入装配：Deps 作为单一入口，Service/Repo/Handler 经构造函数注入；
    禁止在 Handler/Service 中直接 new 外部客户端或直连 DB；支持 HTTP/GRPC/MQ 等多传输共用同一业务服务。
  references:
    - constitution.md
    - dev_crud_http_guides.md

scope:
  codebase:
    deps_file: "internal/app/shared/deps.go"
    service_globs:
      - "internal/service/**/**_service.go"
    repo_globs:
      - "internal/repository/**/**_repo.go"
    handler_globs:
      - "internal/transport/http/**/**_handler.go"
    grpc_server_globs:
      - "internal/transport/grpc/**/**.go"

principles:
  - Deps 为“服务容器”：提供 DB/Logger/Cache/EventBus/KeyRing/Config 等基础依赖。
  - 构造函数注入：NewXxxService(NewXxxRepo(...))；禁止全局单例与包级可变状态。
  - 传输解耦：HTTP/GRPC 层仅持有 Service；Service 不感知传输实现。
  - 生命周期统一：Deps 负责启动/关闭顺序（DB→Repo/Service→Transport）。

checks:
  - id: deps.file.exists
    level: error
    when: { file: "internal/app/shared/deps.go" }
    assert:
      - must_define: "type Deps struct"
      - must_define: "func NewDeps() (*Deps, error)"
  - id: service.constructor
    level: error
    when: { glob: "internal/service/**/**_service.go" }
    assert:
      - must_define_like: "func New*Service(*Deps) *"
      - must_not_import: ["database/sql","gorm.io/gorm"]   # Service 通过 deps 间接持有 DB
  - id: repo.constructor
    level: error
    when: { glob: "internal/repository/**/**_repo.go" }
    assert:
      - must_define_like: "func New*Repo(*gorm.DB) *"
  - id: handler.depends_on_service_only
    level: error
    when: { glob: "internal/transport/http/**/**_handler.go" }
    assert:
      - must_define_like: "type *Handler struct { svc *service.* }"
      - must_define_like: "func New*Handler(*service.*) *"
      - must_not_import: ["gorm.io/gorm","database/sql","net/http"]
  - id: grpc.server.uses_service
    level: warn
    when: { glob: "internal/transport/grpc/**/**.go" }
    assert:
      - should_contain: "deps.*Service"   # gRPC 服务实现注入 Service，而非直连 Repo/DB
  - id: deps.no_new_external_in_handlers
    level: error
    when: { glob: "internal/transport/http/**/**_handler.go" }
    assert:
      - must_not_call: ["redis.NewClient(","kafka.NewReader(","http.DefaultClient.Do"]

acceptance:
  checklist:
    - "[ ] internal/app/shared/deps.go 定义 Deps 与 NewDeps"
    - "[ ] Service/Repo/Handler 均提供 New* 构造函数"
    - "[ ] Handler/GRPC 层仅依赖 Service，不直连 Repo/DB/外部客户端"
    - "[ ] Deps 提供 DB/Logger/Cache/EventBus/KeyRing/Config 等公共依赖"
    - "[ ] 无全局可变单例；关闭顺序由 Deps 统一管理"

templates:
  deps_go: |
    // internal/app/shared/deps.go
    package shared

    import (
      "context"
      "gorm.io/gorm"
      "github.com/redis/go-redis/v9"
      "log"
    )

    type Deps struct {
      DB      *gorm.DB
      Redis   *redis.Client
      Logger  *log.Logger
      // EventBus  any
      // KeyRing   any  // 与 STS 对齐的验签/加密材料
      // Config    *Config
      // Tracer    trace.Tracer
      // ...
      // 业务服务
      {{Entity}}Service *{{domain}}.{{Entity}}Service
    }

    func NewDeps() (*Deps, error) {
      // 1) 初始化基础设施（DB/Cache/Logger/...）
      db := mustOpenDB()
      rdb := mustOpenRedis()
      logger := mustLogger()

      d := &Deps{DB: db, Redis: rdb, Logger: logger}

      // 2) 初始化 Repository & Service
      {{entity}}Repo := {{domain}}repo.New{{Entity}}Repo(d.DB)
      d.{{Entity}}Service = {{domain}}svc.New{{Entity}}Service(d, {{entity}}Repo)

      return d, nil
    }

    func (d *Deps) Close(ctx context.Context) error {
      // 逆序关闭资源
      if d.Redis != nil { _ = d.Redis.Close() }
      // if d.DB != nil { sqlDB, _ := d.DB.DB(); _ = sqlDB.Close() }
      return nil
    }
  service_go: |
    // internal/service/{{domain}}/{{resource}}_service.go
    package {{domain}}svc

    import "github.com/ArtisanCloud/PowerX/internal/app/shared"

    type {{Entity}}Service struct {
      deps *shared.Deps
      repo {{domain}}repo.{{Entity}}Repo
    }

    func New{{Entity}}Service(d *shared.Deps, r {{domain}}repo.{{Entity}}Repo) *{{Entity}}Service {
      return &{{Entity}}Service{deps: d, repo: r}
    }
  repo_go: |
    // internal/repository/{{domain}}/{{resource}}_repo.go
    package {{domain}}repo

    import "gorm.io/gorm"

    type {{Entity}}Repo interface {
      // 定义接口方法
    }

    type {{Entity}}RepoImpl struct {
      db *gorm.DB
    }

    func New{{Entity}}Repo(db *gorm.DB) {{Entity}}Repo {
      return &{{Entity}}RepoImpl{db: db}
    }
  wire_in_api_go: |
    // internal/transport/http/{{layer}}/{{domain}}/api.go
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
