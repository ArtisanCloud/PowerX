# Skills 数据模型与注册治理

本文定义 Skill 的数据结构、状态机与注册治理规则。

## 1. 核心实体

### 1.0 SkillPackageSource

PowerX Skill 的标准源格式是 `SKILL.md` 目录包。数据库记录是解析后的治理态索引，不是源格式替代品。

- `package_id`
- `skill_id`
- `version`
- `source_format`：固定 `skill_package`
- `package_uri`
- `package_path`
- `skill_md_path`
- `raw_markdown`
- `frontmatter_json`
- `body_markdown`
- `input_schema_json`
- `output_schema_json`
- `executor_json`
- `references_manifest_json`
- `package_checksum`
- `imported_at`
- `imported_by`

规则：

1. `SKILL.md` 必须可解析为 `frontmatter_json + body_markdown`。
2. `package_checksum` 覆盖 `SKILL.md`、schema、executor、scripts、references、assets。
3. 导入后的 `SkillRegistryRecord.manifest_json` 必须能追溯到 `SkillPackageSource`。
4. 已发布版本禁止在不变更 version 的情况下覆盖 `raw_markdown/package_checksum`。
5. PowerX Runtime 运行时读取 Registry，不直接读取插件本地包路径。

### 1.1 SkillRegistryRecord

- `skill_id`
- `version`
- `status`：`draft/published/deprecated/disabled`
- `source`：`builtin/plugin/third_party`
- `source_format`：`skill_package|legacy_manifest`
- `package_source_id`
- `bundle_uri`
- `checksum`
- `signature`
- `manifest_json`
- `raw_markdown`
- `frontmatter_json`
- `body_markdown`
- `capability_binding`
- `created_at`
- `updated_at`

### 1.2 SkillExecutionTrace

- `trace_id`
- `tenant_uuid`
- `skill_id`
- `version`
- `entrypoint`
- `protocol`（固定 `skill`）
- `status`
- `latency_ms`
- `error_summary`
- `request_payload`
- `response_payload`
- `created_at`

## 2. 状态机

1. `draft` -> `published`
2. `published` -> `deprecated`
3. `published` -> `disabled`
4. `deprecated` -> `disabled`
5. `published` -> `published`（新版）

## 3. 版本与回滚策略

1. 允许同 `skill_id` 多版本并存。
2. 每个 skill 只有一个 `latest published`。
3. 回滚通过切换指针，不删除历史版本。
4. 禁止覆盖已发布版本内容（不可变版本）。

## 4. 索引建议

1. 唯一索引：`(skill_id, version)`
2. 查询索引：`(status, source, updated_at desc)`
3. 租户查询：`(tenant_uuid, skill_id, created_at desc)` for trace

## 5. 与 Capability Registry 关系

1. 一个 Skill 可绑定一个或多个 capability。
2. capability 与 skill 版本建议显式映射，避免隐式漂移。
3. `preferred_protocol=skill` 时走 SkillAdapter。

## 6. 数据一致性约束

1. `status=published` 时 `checksum` 必填。
2. `source=third_party` 时 `source_uri` 必填。
3. `signature` 可选，但企业策略可配置为必填。
