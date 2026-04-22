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

### Context Optimization Extensions
- **新增字段**:
  - `prompt_tokens`
  - `completion_tokens`
  - `cached_tokens`（provider 支持时）
  - `context_layers_size`（L0-L5 token 分布）
  - `trim_actions`（裁剪步骤与原因）
  - `cache_mode`（`auto|force_off|force_on`）
  - `cache_hit`（bool）
- **规则**:
  - 调用失败也必须尽量记录 token/trim 观测字段（可空但需保留字段语义）
  - `trim_actions` 需可回放，便于定位“为何丢失上下文”

## AgentChatSession Summary Schema（扩展）
- **目标**: 从纯文本摘要升级为结构化摘要。
- **建议结构**:
  - `facts[]`
  - `decisions[]`
  - `open_issues[]`
  - `constraints[]`
  - `updated_at`
- **兼容策略**:
  - 旧文本摘要可继续读取
  - 新写入优先结构化 JSON，必要时降级为文本镜像

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

## AgentTeam
- **标识**: `team_id`
- **核心字段**:
  - `tenant_uuid`
  - `parent_agent_id`
  - `team_name`
  - `dispatch_mode`（`serial|parallel|mixed`）
  - `default_failure_policy`（`fail-fast|continue|retry-once`）
  - `status`（`active|disabled`）
  - `created_by`, `created_at`, `updated_at`
- **规则**:
  - `team_id` 仅在单租户作用域内有效
  - `parent_agent_id` 必须属于同租户

## AgentTeamMember
- **标识**: `team_id` + `child_agent_id`
- **核心字段**:
  - `role`（`planner|retriever|executor|reviewer`）
  - `priority`
  - `enabled`
  - `created_at`, `updated_at`
- **规则**:
  - 同一 `team_id` 下 `child_agent_id` 唯一
  - 禁止跨租户 agent 加入同一 team

## AgentHandoffTask
- **标识**: `task_id`
- **核心字段**:
  - `team_id`
  - `tenant_uuid`
  - `parent_agent_id`
  - `child_agent_id`
  - `session_id`
  - `plan_id`
  - `node_id`
  - `context_ref`（引用切片，不存整段大上下文）
  - `input_digest`
  - `output_digest`
  - `failure_policy`（`fail-fast|continue|retry-once`）
  - `status`（`queued|running|completed|failed|timed_out|cancelled`）
  - `error_code`, `error_summary`
  - `started_at`, `ended_at`, `created_at`
- **规则**:
  - 必须记录 `parent_agent_id` 与 `child_agent_id`
  - 失败也必须保留 `error_code/error_summary` 便于回放

## AgentSharedContextRef
- **标识**: `context_ref_id`
- **核心字段**:
  - `tenant_uuid`
  - `session_id`
  - `owner_agent_id`
  - `visible_to_agent_ids[]`
  - `payload_uri`（对象存储或内部文档 URI）
  - `checksum`
  - `expires_at`
  - `created_at`
- **规则**:
  - 子 Agent 仅能访问 `visible_to_agent_ids` 内授权引用
  - 过期引用不可继续参与 handoff

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
