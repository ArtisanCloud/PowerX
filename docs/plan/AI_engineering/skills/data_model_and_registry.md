# Skills 数据模型与注册治理

本文定义 Skill 的数据结构、状态机与注册治理规则。

## 1. 核心实体

### 1.1 SkillRegistryRecord

- `skill_id`
- `version`
- `status`：`draft/published/deprecated/disabled`
- `source`：`builtin/plugin/third_party`
- `bundle_uri`
- `checksum`
- `signature`
- `manifest_json`
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

