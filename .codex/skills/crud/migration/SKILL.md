---
name: crud-migration
description: PowerX CRUD Migration 规则（集中迁移挂载与入口约束）。
---

# PowerX CRUD Migration

## 步骤

1) 打开 `本文件内嵌规则`。
2) 按规则执行实现/校对。
3) 完成后按核对清单验收。

## 核对点

- 与 PowerX 当前代码结构、路径与命名一致。
- 仅在传输层/契约层做职责内改动，不跨层越界。

## 规则（内嵌）

### migration.yaml

````yaml
kind: ruleset
name: crud_migration
version: 1.2.0
owner: powerx
status: stable

meta:
  intent: >
    约束 CoreX 数据库迁移的集中式挂载模式：
    - 总入口仅在 cmd/database/migrate.go 定义 MigrateDatabase/ResetDatabase（负责编排，不直接写模型迁移逻辑）。
    - 所有 CoreX 模型迁移必须集中在 pkg/corex/db/database/migration.go，通过 MigrateCoreModels（及内部 migrate<Domain>Models）统一调用 GORM AutoMigrate/Migrator。
    - plan.md 与 tasks.md 需显式说明增量模型将挂载到 pkg/corex/db/database/migration.go；禁止生成 pkg/corex/db/migration/<domain> 之类的分散入口或裸 .sql。
    - 若与以上冲突，以本块为准；否则视为失败。

scope:
  codebase:
    migrate_entry: "cmd/database/migrate.go"
    core_migration_file: "pkg/corex/db/database/migration.go"
    legacy_glob: "pkg/corex/db/migration/**"

principles:
  - “入口 + 集中挂载”双层结构：入口统一编排；具体 AutoMigrate 均集中在 pkg/corex/db/database/migration.go 内部函数。
  - 默认 Postgres：重置以 schema 为粒度，schema 名来自 model.PowerXSchema。
  - 允许 MySQL 降级：逐表 DROP（关闭外键检查）作为兼容路径。
  - 不交付裸 .sql 文件；优先 AutoMigrate/Migrator。
  - 生产保护：重置操作需具备显式的环境保护（如 APP_ENV != production）。

checks:
  # ---- 入口文件存在与函数签名 ----
  - id: migrate.entry.exists
    level: error
    when: { file: "cmd/database/migrate.go" }
    assert:
      - must_define: "func MigrateDatabase(ctx context.Context, db *gorm.DB) error"
      - must_define: "func ResetDatabase(ctx context.Context, db *gorm.DB) error"

  # ---- 入口逻辑：必须调用核心与 agent 模块迁移 ----
  - id: migrate.entry.calls.core_and_agent
    level: error
    when: { file: "cmd/database/migrate.go" }
    assert:
      - contains: "database.MigrateCoreModels(db)"
      - contains: "persistence.MigrateAgentModels(db)"

  # ---- ResetDatabase：必须使用 PowerXSchema 且包含 PG 的 DROP/CREATE 语句 ----
  - id: migrate.reset.uses_schema_constant
    level: error
    when: { file: "cmd/database/migrate.go" }
    assert:
      - contains: "model.PowerXSchema"
  - id: migrate.reset.pg_drop_create
    level: error
    when: { file: "cmd/database/migrate.go" }
    assert:
      - contains_any:
          - 'DROP SCHEMA " + model.PowerXSchema + " CASCADE; CREATE SCHEMA " + model.PowerXSchema + ";"'
          - "DROP SCHEMA "
          - "CASCADE; CREATE SCHEMA "

  # ---- ResetDatabase：允许 MySQL 逐表删除作为降级方案（可选）----
  - id: migrate.reset.mysql_fallback
    level: warn
    when: { file: "cmd/database/migrate.go" }
    assert:
      - contains_any:
          - "SHOW TABLES"
          - "SET FOREIGN_KEY_CHECKS=0"
          - "DROP TABLE IF EXISTS"

  # ---- 生产环境保护 ----
  - id: migrate.reset.env_guard
    level: warn
    when: { file: "cmd/database/migrate.go" }
    assert:
      - contains_any: ["APP_ENV", "production", "ensureNotProduction"]

  # ---- CoreX 迁移集中定义 ----
  - id: migrate.core.registry
    level: error
    when: { file: "pkg/corex/db/database/migration.go" }
    assert:
      - must_define: "func MigrateCoreModels(db *gorm.DB) (err error)"
      - must_call_one_of: ["db.AutoMigrate(", "db.Migrator()."]
      - contains: "func migrate"

  # ---- 避免遗留的 pkg/corex/db/migration/<domain> 目录 ----
  - id: migrate.no_legacy_packages
    level: warn
    when: { glob: "pkg/corex/db/migration/**/migrate.go" }
    assert:
      - should_empty: true

  # ---- 避免散落裸 SQL 文件 ----
  - id: migration.no_raw_sql_files
    level: warn
    when: { glob: "**/*.sql" }
    assert:
      - should_empty: true

acceptance:
  checklist:
    - "[ ] cmd/database/migrate.go 定义了 MigrateDatabase 与 ResetDatabase"
    - "[ ] MigrateDatabase 依次调用 database.MigrateCoreModels 与 persistence.MigrateAgentModels"
    - "[ ] ResetDatabase 使用 model.PowerXSchema 并包含 PG 的 DROP SCHEMA ... CASCADE; CREATE SCHEMA ... 实现"
    - "[ ] （可选）MySQL 降级路径：SHOW TABLES + 逐表 DROP + FK 开关"
    - "[ ] pkg/corex/db/database/migration.go 中新增或更新 migrate<Domain>Models，并通过 AutoMigrate/Migrator 挂载"
    - "[ ] 无裸 .sql 交付；生产环境有 reset 保护；未新增 pkg/corex/db/migration/<domain> 之类入口"

templates:
  migrate_entry_go_hint: |
    // cmd/database/migrate.go 的关键片段（示意）：
    func MigrateDatabase(ctx context.Context, db *gorm.DB) error {
      if err := database.MigrateCoreModels(db); err != nil { return err }
      if err := persistence.MigrateAgentModels(db); err != nil { return err }
      return nil
    }
    func ResetDatabase(ctx context.Context, db *gorm.DB) error {
      // Postgres：DROP SCHEMA <schema> CASCADE; CREATE SCHEMA <schema>;
      // MySQL（可选）：SHOW TABLES → 关闭 FK → 逐表 DROP → 恢复 FK
      return nil
    }
  migrate_core_registry_hint: |
    // pkg/corex/db/database/migration.go 的增量挂载示意：
    func MigrateCoreModels(db *gorm.DB) (err error) {
      if err = db.AutoMigrate(&modelTenant.Tenant{}); err != nil { return err }
      if err = migrateCapabilityModels(db); err != nil { return err }
      return nil
    }
    func migrateCapabilityModels(db *gorm.DB) error {
      return db.AutoMigrate(&modelCapability.CapabilityContract{})
    }
````
