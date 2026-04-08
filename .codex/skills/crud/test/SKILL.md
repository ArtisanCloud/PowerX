---
name: crud-test
description: PowerX CRUD 测试规则（Service 必测、Repo 可选集测）。
---

# PowerX CRUD Test

## 步骤

1) 打开 `本文件内嵌规则`。
2) 按规则执行实现/校对。
3) 完成后按核对清单验收。

## 核对点

- 与 PowerX 当前代码结构、路径与命名一致。
- 仅在传输层/契约层做职责内改动，不跨层越界。

## 规则（内嵌）

### test.yaml

````yaml

kind: ruleset
name: crud_test
version: 1.0.0
owner: powerx
status: stable

meta:
  intent: >
    要求 CRUD 关键层有最小可用测试覆盖：Service 单元测试必须存在；
    可选提供 Repository 的集成测试（sqlite/pg 临时库）。
  references:
    - crud_service.yaml
    - crud_repository.yaml

scope:
  applies_to:
    - "internal/service/**/**_service_test.go"
    - "internal/repository/**/**_repo_test.go"

principles:
  - Service 层单测为必选：mock Repo，覆盖 404/409/成功分支与事务失败。
  - Repository 可选集测：使用临时 DB（prefer sqlite-in-memory 或 testcontainer PG）。
  - 测试不依赖传输层（无 gin/grpc 依赖）。

checks:

  # Service 单测存在
  - id: service.tests.exist
    level: error
    when: { glob: "internal/service/**/**_service.go" }
    assert:
      - must_have_companion_test: true

  # Service 测试基础要素
  - id: service.tests.shape
    level: warn
    when: { glob: "internal/service/**/**_service_test.go" }
    assert:
      - must_import: ["testing"]
      - should_import_any: ["github.com/stretchr/testify/assert","github.com/stretchr/testify/require"]
      - must_not_import: ["github.com/gin-gonic/gin","google.golang.org/grpc"]

  # Repository 集测（可选，存在则建议约束）
  - id: repo.tests.optional
    level: warn
    when: { glob: "internal/repository/**/**_repo_test.go" }
    assert:
      - should_import_any: ["gorm.io/driver/sqlite","github.com/testcontainers/testcontainers-go"]
      - should_contain_any: ["sqlite.Open(\"file::memory:?cache=shared\")","testcontainers"]

acceptance:
  checklist:
    - "[ ] 每个 Service 至少有一个 *_service_test.go 覆盖核心分支（404/409/成功/事务失败）"
    - "[ ] 测试不依赖 HTTP/gRPC 传输"
    - "[ ] 有基础断言库（testify）"
    - "[ ] （可选）Repository 集测基于临时 DB"

templates:
  service_test_go: |
    // internal/service/{{domain}}/{{resource}}_service_test.go
    package {{domain}}svc_test

    import (
      "context"
      "errors"
      "testing"

      "{{module_path}}/internal/app/shared"
      svc "{{module_path}}/internal/service/{{domain}}"
      repo "{{module_path}}/internal/repository/{{domain}}"
      "github.com/stretchr/testify/assert"
      "gorm.io/gorm"
    )

    // 伪造 Repo（可替换为 gomock）
    type fakeRepo struct {
      create func(ctx context.Context, db *gorm.DB, tenantID uint64, in any) error
      get    func(ctx context.Context, db *gorm.DB, tenantID uint64, id string) (any, error)
    }
    // 满足接口（示例，按你实际接口补全）
    // ...

    func Test_Get_NotFound(t *testing.T) {
      d := &shared.Deps{}
      fr := &fakeRepo{
        get: func(ctx context.Context, db *gorm.DB, tenantID uint64, id string) (any, error) {
          return nil, gorm.ErrRecordNotFound
        },
      }
      s := svc.New{{Entity}}Service(d, fr)
      _, err := s.Get(context.Background(), 1, "id-x")
      assert.Error(t, err)
      assert.Equal(t, svc.ErrNotFound, err)
    }

  repo_sqlite_test_go: |
    // internal/repository/{{domain}}/{{resource}}_repo_test.go
    package {{domain}}repo_test

    import (
      "context"
      "testing"

      "gorm.io/driver/sqlite"
      "gorm.io/gorm"
      repo "{{module_path}}/internal/repository/{{domain}}"
      m "{{module_path}}/pkg/corex/db/persistence/model/{{domain}}"
      "github.com/stretchr/testify/require"
    )

    func newDB(t *testing.T) *gorm.DB {
      db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
      require.NoError(t, err)
      require.NoError(t, db.AutoMigrate(&m.{{Entity}}{}))
      return db
    }

    func Test_Create_And_Get(t *testing.T) {
      db := newDB(t)
      r := repo.New{{Entity}}Repo(db)
      ctx := context.Background()

      in := &m.{{Entity}}{TenantID: 1, Name: "n"}
      require.NoError(t, r.Create(ctx, db, 1, in))

      got, err := r.GetByID(ctx, db, 1, in.ID)
      require.NoError(t, err)
      require.NotNil(t, got)
    }
````
