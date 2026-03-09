# Skills Persistence Models

本目录定义 Skills 领域持久化模型：
- SkillRegistryRecord
- OfficialSkillCatalogEntry
- SkillCapabilityBinding
- SkillExecutionTrace
- SkillLifecycleAudit

约束：
- 模型字段命名与 data-model.md 保持一致。
- 迁移统一在 `pkg/corex/db/database/migration.go` 挂载。
