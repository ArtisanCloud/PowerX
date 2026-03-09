# Data Model — PowerX Skills 管理与治理

## SkillRegistryRecord
- **标识**: `skill_id` + `version`（唯一）
- **核心字段**:
  - `source` (`builtin|plugin|third_party`)
  - `status` (`draft|published|deprecated|disabled`)
  - `is_latest_published`（同 skill_id 仅一个 true）
  - `bundle_uri`
  - `checksum`
  - `signature`（可空）
  - `manifest_json`
  - `source_url`（可空，仅追溯）
  - `source_ref`（可空，仅追溯）
  - `import_type`（首版固定 `upload`）
  - `created_by`, `created_at`, `updated_at`
- **规则**:
  - 已发布版本不可覆盖内容
  - `published` 必须有有效 `checksum`
  - 当策略开启时，`published` 需有效 `signature`

## OfficialSkillCatalogEntry
- **标识**: `catalog_skill_id`
- **核心字段**:
  - `skill_id`
  - `recommended_version`
  - `risk_level` (`L1|L2|L3|L4`)
  - `category`
  - `summary`
  - `active`
  - `updated_at`
- **规则**:
  - 仅由平台版本维护更新
  - 与 Registry 解耦，允许“目录有但未安装”

## SkillCapabilityBinding
- **标识**: `skill_id` + `version` + `capability_id`
- **核心字段**:
  - `tool_grants[]`
  - `visibility_scope`
  - `created_by`, `created_at`
- **规则**:
  - 绑定需在 `published` 后生效
  - skill 下线/禁用需触发绑定有效性检查

## SkillExecutionTrace
- **标识**: `trace_id`
- **核心字段**:
  - `tenant_uuid`
  - `skill_id`
  - `version`
  - `entrypoint`
  - `protocol_used`（`skill`）
  - `invoke_path`（`tenant.skills.invoke|tenant.invocations`）
  - `status` (`completed|failed|denied`)
  - `latency_ms`
  - `error_code`
  - `error_summary`
  - `request_payload_digest`
  - `response_payload_digest`
  - `created_at`
- **规则**:
  - 查询必须受租户隔离约束
  - 关键字段不得为空（trace_id/tenant_uuid/skill_id/version/status）

## SkillLifecycleAudit
- **标识**: `audit_id`
- **核心字段**:
  - `action` (`import|publish|rollback|disable|bind_capability`)
  - `skill_id`
  - `version`
  - `operator`
  - `tenant_scope`
  - `reason`
  - `result`
  - `created_at`
- **规则**:
  - 每次关键动作必须写审计
  - 支持按 trace_id 或 skill/version 关联检索

## 状态机与并发约束
- **状态迁移**:
  - `draft -> published`
  - `published -> deprecated`
  - `published -> disabled`
  - `deprecated -> disabled`
- **发布/回滚约束**:
  - 同一 `skill_id` 同时仅允许一个 `latest_published`
  - 回滚通过切换指针，不删除历史
  - 并发发布/回滚需互斥，避免双 latest
