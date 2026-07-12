# Data Model — PowerX Skills 管理与治理

## SkillPackageSource
- **标识**: `package_id`
- **作用**: 保存 `SKILL.md` 目录包的源格式快照；数据库 Registry 是治理态索引，不替代源格式。
- **核心字段**:
  - `skill_id`
  - `version`
  - `source_format`（固定 `skill_package`）
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
  - `imported_by`
  - `imported_at`
- **规则**:
  - `SKILL.md` 必须包含 YAML frontmatter 与 Markdown body。
  - schema/executor 可以内联，也可以引用包内相对路径；禁止越权引用包外文件。
  - `package_checksum` 必须覆盖 `SKILL.md`、schema、executor、scripts、references、assets。
  - 已发布版本不可在不变更 version 的情况下覆盖 `raw_markdown/package_checksum`。

## SkillRegistryRecord
- **标识**: `skill_id` + `version`（唯一）
- **核心字段**:
  - `source` (`builtin|plugin|third_party`)
  - `source_format` (`skill_package|legacy_manifest`)
  - `package_source_id`
  - `status` (`draft|published|deprecated|disabled`)
  - `is_latest_published`（同 skill_id 仅一个 true）
  - `bundle_uri`
  - `checksum`
  - `signature`（可空）
  - `manifest_json`
  - `raw_markdown`
  - `frontmatter_json`
  - `body_markdown`
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

## PluginSkillDefinition
- **标识**: `provider_plugin_id` + `skill_id` + `version`
- **核心字段**:
  - `provider_plugin_id`
  - `skill_id`
  - `version`
  - `title`
  - `description`
  - `intent_examples_json`
  - `input_schema_json`
  - `output_schema_json`（可空）
  - `prompt_refs_json`（可空）
  - `executor_json`
  - `source_bundle_uri`
  - `checksum`
  - `discovered_at`
  - `imported_registry_id`（可空，指向治理态 Skill）
- **规则**:
  - 插件源定义不得直接作为 Agent 候选池权威源。
  - 必须导入为 `SkillRegistryRecord(source=plugin)` 并发布后才可被调用。
  - 同一插件内 `skill_id + version` 唯一，跨插件同名需依赖 `provider_plugin_id` 消歧。

## PluginSkillSource
- **标识**: `provider_plugin_id` + `plugin_skill_id` + `version`
- **作用**: 表示 PowerX 底座中某条治理态 Skill 与插件自有 Skill Local 的来源映射。
- **核心字段**:
  - `provider_plugin_id`
  - `plugin_skill_id`
  - `powerx_skill_id`
  - `version`
  - `tenant_uuid`
  - `manifest_snapshot_json`
  - `executor_json`
  - `capability_id`
  - `checksum`
  - `sync_action`（`create|update|publish|disable|refresh`）
  - `sync_status`（`pending|synced|failed|drifted|disabled`）
  - `sync_error`
  - `last_sync_at`
  - `created_at`, `updated_at`
- **规则**:
  - `powerx_skill_id` 指向 PowerX 治理态 Skill，Agent Runtime 只读取治理态 Skill。
  - `manifest_snapshot_json/checksum` 用于检测插件自有声明与 PowerX 治理态记录是否漂移。
  - `sync_status=failed|drifted` 时不得进入 Agent 候选池，也不得作为 Agent 绑定目标。

## PluginAgentSource
- **标识**: `provider_plugin_id` + `plugin_agent_id`
- **作用**: 表示 PowerX 底座中某个 Agent 与插件自有 Agent Local 的来源映射。
- **核心字段**:
  - `provider_plugin_id`
  - `plugin_agent_id`
  - `powerx_agent_uuid`
  - `tenant_uuid`
  - `agent_key`
  - `name`
  - `description`
  - `model_profile_ref`
  - `prompt_snapshot`
  - `bound_powerx_skill_ids[]`
  - `local_skill_refs[]`
  - `sync_action`（`create|update|bind|disable|refresh`）
  - `sync_status`（`pending|synced|failed|drifted|disabled`）
  - `sync_error`
  - `last_sync_at`
  - `created_at`, `updated_at`
- **规则**:
  - `powerx_agent_uuid` 指向 PowerX 运行态 Agent；创建 Agent Session 时必须使用该 UUID。
  - 绑定 Skill 时只能引用已发布、已同步且当前租户可见的 PowerX Skill。
  - 插件 Agent Local 未同步成功时，PowerX 不得为其创建会话或执行计划。

## PluginRegistrySyncAudit
- **标识**: `audit_id`
- **核心字段**:
  - `provider_plugin_id`
  - `tenant_uuid`
  - `plugin_agent_id`
  - `plugin_skill_id`
  - `powerx_agent_uuid`
  - `powerx_skill_id`
  - `sync_action`
  - `sync_status`
  - `operator`
  - `trace_id`
  - `error_code`
  - `error_summary`
  - `created_at`
- **规则**:
  - 每次插件 Plugin Registry 同步请求都必须写审计。
  - 同步审计需可与 Agent Run Trace、SkillLifecycleAudit 通过 `trace_id/provider_plugin_id` 串联。

## PluginSkillInvocationContext
- **标识**: `trace_id`
- **核心字段**:
  - `tenant_uuid`
  - `user_uuid`
  - `agent_id`
  - `session_id`
  - `message_id`
  - `channel`
  - `locale`
  - `skill_id`
  - `provider_plugin_id`
  - `capability_id`
  - `trace_id`
  - `idempotency_key`
- **规则**:
  - `tenant_uuid/user_uuid/agent_id/session_id/skill_id/trace_id` 不得为空。
  - 插件 executor 必须以该上下文为执行边界，禁止从 payload 中自行推断租户或用户。
  - 缺少上下文时返回 fail-fast 错误，不允许匿名 fallback。

## PluginSkillInvocationTrace
- **标识**: `trace_id` + `provider_plugin_id` + `skill_id`
- **核心字段**:
  - `tenant_uuid`
  - `provider_plugin_id`
  - `skill_id`
  - `version`
  - `agent_id`
  - `session_id`
  - `message_id`
  - `executor_path`
  - `capability_id`
  - `status`（`queued|running|completed|failed|denied`）
  - `plugin_task_id`
  - `error_code`
  - `error_summary`
  - `latency_ms`
  - `created_at`
  - `updated_at`
- **规则**:
  - 需与 `SkillExecutionTrace` 通过同一 `trace_id` 可关联。
  - 插件返回业务任务 ID 时写入 `plugin_task_id`，用于后续状态查询与回调。
  - capability 不匹配、插件未安装、executor 不可用均必须记录为拒绝或失败事件。

## PowerXPluginAgentClientConfig
- **标识**: `provider_plugin_id` + `tenant_uuid`
- **核心字段**:
  - `gateway_base_url`
  - `agent_invoke_path`
  - `agent_sse_path`
  - `agent_ws_path`
  - `sts_client_id_ref`
  - `auth_scheme`（固定 `bearer`）
  - `enabled`
  - `created_at`, `updated_at`
- **规则**:
  - 用于 PowerXPlugin Framework Client 访问 PowerX Agent Session/Stream。
  - delegated 模式下凭证来自 STS，不允许使用已废弃的静态插件 token。
  - 插件自有 Chat 必须使用该配置，不维护私有 Agent endpoint。

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

## ResponseMode
- **标识**: `response_mode`
- **枚举值**:
  - `capability_intro`
  - `capability_howto`
  - `skill_execution`
  - `clarify_params`
  - `normal_chat`
  - `error_explain`
- **规则**:
  - 只能由 ResponsePlanner 输出或由 Runtime error handler 明确设置。
  - `capability_intro/capability_howto` 不代表一定执行 Skill。
  - `skill_execution` 必须能关联 planner task、tool call 或 executor result。
  - `clarify_params` 必须包含缺失字段。
  - `normal_chat` 默认不注入完整能力目录。

## ResponsePlan
- **标识**: `response_plan_id`
- **作用**: 表示一轮 Agent Message Run 在最终回复前的结构化回答计划。
- **核心字段**:
  - `response_plan_id`
  - `tenant_uuid`
  - `agent_id`
  - `session_id`
  - `message_id`
  - `run_id`
  - `trace_id`
  - `response_mode`
  - `response_intents[]`
  - `answer_requirements[]`
  - `should_call_tool`
  - `target_capability_ids[]`
  - `use_capability_context`
  - `include_examples`
  - `include_schema`
  - `repeat_full_intro`
  - `needs_clarification`
  - `missing_fields[]`
  - `reason`
  - `model_selection`
  - `created_at`
- **规则**:
  - `response_mode` 是主回答模式，负责选择上下文策略。
  - `response_intents[]` 是同一条用户消息里的可组合意图，例如 `greeting/agent_identity/capability_intro/test_recommendation` 可以同时存在。
  - `answer_requirements[]` 是最终回复必须满足的回答要求；`recent_capability_intro` 只能影响展开程度，不能删除当前消息要求回答的问题。
  - `target_capability_ids[]` 必须来自当前 Agent 已绑定、已发布、租户可见且权限通过的能力集合。
  - `reason` 只进入 trace/debug，不直接展示给终端用户。
  - ResponsePlan 可以作为 Trace artifact，也可以将摘要写入 message meta；不得在普通 message meta 中保存完整 prompt 或敏感上下文。

## ResponseGuidance

- **标识**: `response_guidance`
- **作用**: 表示 Agent/Skill 给 Final Response 的表达规范材料。它不是 Core Runtime 的业务分支，而是由 Agent 配置和 Skill metadata 提供的数据。
- **来源**:
  - Agent `persona`
  - Agent `prompt_seed`
  - Skill `manifest_json.response_guidance`
  - Skill `manifest_json.prompt_spec.response_guidance`
- **标准分组**:
  - `general`
  - `capability_intro`
  - `capability_howto`
  - `clarify_params`
  - `skill_execution`
  - `error_explain`
- **运行态映射**:
  - `ToolCallCandidate.ResponseGuidance`
  - `CapabilityContextItem.ResponseGuidance`
  - `[CONTEXT-L1 CAPABILITIES]` 的 `回复规范`
- **规则**:
  - Core Runtime 只负责抽取、去重、保留 mode 标签和拼装上下文。
  - 业务字段规则、执行话术、行业示例必须写入 Agent/Skill 数据，不得写进 Core prompt 代码。
  - `response_guidance` 不得携带租户、用户、token、session 或权限判断等运行时身份信息。
  - `general` 可作为所有 mode 的通用规范；其他分组必须按 `mode: text` 形式进入候选能力上下文。

## AssistantMessageMeta
- **标识**: `message_id`
- **作用**: assistant 消息的结构化 metadata，用于上下文去重、追问、质量评估和 Trace 关联。
- **核心字段**:
  - `response_mode`
  - `capability_ids[]`
  - `response_plan_id`
  - `used_context_layers[]`
  - `tool_calls[]`
  - `final_response_model`
  - `model_selection`
  - `trace_id`
  - `run_id`
  - `plan_id`
- **规则**:
  - 去重判断必须基于 `response_mode/capability_ids` 等 meta，不得依赖自然语言文本匹配。
  - `capability_ids[]` 只能包含当前 Agent 可见能力。
  - 不得保存完整 executor payload、原始 prompt 或敏感 context；这些内容必须进入受控 artifact。

## AgentContextDriver
- **作用**: 定义 Agent Runtime 中上下文来源的职责边界。
- **驱动分层**:
  - `runtime_memory`: 本轮请求过程态，例如 ResponsePlan、节点输出、短生命周期执行状态。
  - `postgres`: 权威源，保存 session/message/message meta、Skill Registry、Agent-Skill Binding、模型策略、结构化摘要、context_ref metadata。
  - `redis`: 短 TTL 缓存，保存 planner decision、response_plan、候选快照、recent meta hot window。
  - `local_file`: 本地 Agent Trace artifact，路径为 `backend/logs/agents/{tenant_uuid}/{session_id}/{message_id}`。
  - `loki`: 生产 Agent Trace 事件检索源。
  - `object_storage`: 大型 prompt/context/tool payload artifact，DB 保存 URI/checksum。
- **规则**:
  - Runtime Memory 不得作为权限、候选、会话或 message meta 的唯一事实来源。
  - Redis key 必须包含租户、Agent、Session、候选指纹或策略版本，禁止跨租户复用。
  - Context Builder 必须以 DB 权威记录为准，Redis 命中只能加速。
  - Trace/Report 存储不替代业务会话主数据。
  - PostgreSQL 默认保存 session/message/message meta、结构化摘要、context_ref、checksum、artifact URI 与配置；完整 prompt、完整上下文正文、tool payload、executor result 不默认入库。
  - Root 调试页面的上下文状态面板读取 message meta 与 Agent Trace 节点摘要；完整上下文正文必须通过受控 Trace artifact、Loki body 或对象存储引用下载。

## AgentChatSession Summary Schema（扩展）
- **目标**: 从纯文本摘要升级为结构化摘要。
- **建议结构**:
  - `schema`
  - `facts[]`
  - `decisions[]`
  - `open_issues[]`
  - `constraints[]`
  - `source_summary_ids[]`
  - `from_message_id`
  - `to_message_id`
  - `compressed_messages`
  - `recent_messages_kept`
  - `compression_policy`
  - `previous_summary_at`
  - `last_compressed_trace_id`
  - `updated_at`
- **首版持久化**:
  - 每次压缩必须插入 `agent_chat_context_summaries`。
  - 当前 active summary 快照保存在 `agent_chat_sessions.summary`。
  - `agent_chat_sessions.summary_at` 保存最新摘要更新时间。
  - `agent_chat_sessions.meta.active_context_summary_id` 指向当前 active summary 对应的压缩记录。
  - 被 active summary 覆盖的非 pinned 旧消息可以从 `agent_chat_messages` 删除，以控制业务表体积。
  - 完整上下文正文、完整 prompt 与 executor payload 不进入 `summary`。
- **表职责**:
  - `agent_chat_context_summaries` 是压缩历史、审计、报告和回放事实来源。
  - `agent_chat_sessions.summary` 是运行时 active snapshot，不承载历史版本职责。
- **兼容策略**:
  - 旧文本摘要可继续读取
  - 新写入优先结构化 JSON，必要时降级为文本镜像

## AgentChatContextSummary
- **表名**: `agent_chat_context_summaries`
- **作用**: 保存每次 Agent 会话上下文压缩记录，与 `agent_chat_sessions.summary` 的 active snapshot 配合使用。
- **核心字段**:
  - `summary_id`
  - `source_summary_id`
  - `tenant_uuid`
  - `session_id`
  - `agent_id`
  - `user_id`
  - `schema`
  - `from_message_id`
  - `to_message_id`
  - `compressed_messages`
  - `recent_messages_kept`
  - `compression_policy`
  - `summary_json`
  - `summary_text`
  - `checksum`
  - `artifact_uri`
  - `meta`
- **规则**:
  - 每次 compact 必须新增一条记录，不覆盖旧记录。
  - `summary_id` 必须写回 `agent_chat_sessions.meta.active_context_summary_id`。
  - `summary_json` 保存结构化摘要；`summary_text` 是可读镜像，不作为唯一事实源。
  - `artifact_uri` 只保存受控 artifact 引用，不直接存完整 prompt 或敏感 payload。

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

## AgentRunTrace
- **标识**: `run_id`
- **作用**: 表示一轮 Agent Message Run 的结构化总览。
- **核心字段**:
  - `trace_id`
  - `tenant_uuid`
  - `user_uuid`
  - `agent_id`
  - `session_id`
  - `message_id`
  - `run_id`
  - `plan_id`
  - `channel`（`web|telegram|discord|wechat|scrm|plugin_registry_chat|api`）
  - `status`（`running|completed|failed|cancelled`）
  - `node_count`
  - `event_count`
  - `error_count`
  - `warning_count`
  - `duration_ms`
  - `user_message_digest`
  - `final_response_digest`
  - `artifact_root`
  - `started_at`, `ended_at`, `created_at`
- **规则**:
  - `run_id` 必须在每轮 Agent Message Run 开始时创建。
  - `tenant_uuid/session_id/message_id/run_id` 必须完整，否则 Trace Logger 必须 fail-fast。
  - 本地文件路径按 `tenant_uuid/session_id/message_id` 分区。
  - 不直接存储完整敏感 prompt；完整内容需进入受控 artifact。

## AgentTraceEvent
- **标识**: `event_id`
- **作用**: Agent Runtime 时间线中的一条结构化事件。
- **核心字段**:
  - `trace_id`
  - `run_id`
  - `tenant_uuid`
  - `agent_id`
  - `session_id`
  - `message_id`
  - `plan_id`
  - `node_id`
  - `node_seq`
  - `node_kind`
  - `node_ref`
  - `phase`（`start|end|error|delta`）
  - `status`（`running|success|error|skipped`）
  - `duration_ms`
  - `input_digest`
  - `output_digest`
  - `artifact_refs[]`
  - `error_code`
  - `error_summary`
  - `created_at`
- **规则**:
  - 每个关键节点至少有 `start` + `end/error`。
  - `timeline.jsonl` 按 `created_at/node_seq` 可稳定排序。
  - Loki Sink 中仅低基数字段进入 label，高基数字段进入 body。

## AgentTraceNodeSnapshot
- **标识**: `run_id + node_id`
- **作用**: 单个 Runtime 节点的可回放快照。
- **核心字段**:
  - `node_id`
  - `node_seq`
  - `node_kind`
  - `node_ref`
  - `phase_status`
  - `input_summary`
  - `output_summary`
  - `context_ref`
  - `skill_id`
  - `plugin_id`
  - `capability_id`
  - `executor_path`
  - `prompt_tokens`
  - `completion_tokens`
  - `cached_tokens`
  - `trim_actions`
  - `error_code`
  - `error_summary`
  - `started_at`, `ended_at`
- **规则**:
  - `nodes/{node_seq}_{node_kind}.json` 是本地文件 sink 的权威节点快照。
  - 节点快照可引用 `artifacts/*`，但必须遵守脱敏和大小限制。

## AgentTraceArtifact
- **标识**: `artifact_id`
- **核心字段**:
  - `run_id`
  - `node_id`
  - `artifact_kind`（`prompt|context|tool_payload|executor_result|response|error_stack`）
  - `uri`
  - `checksum`
  - `redaction_policy`（`redacted|summary|raw_root_only`）
  - `size_bytes`
  - `created_at`
- **规则**:
  - 默认策略为 `redacted`。
  - 超过 `max_artifact_bytes` 的 artifact 必须截断或拒绝写入，并记录 warning 事件。
  - `raw_root_only` 仍需 root 权限，且不得进入普通审计导出。

## AgentRunReport
- **标识**: `report_id`
- **核心字段**:
  - `report_scope`（`message|session`）
  - `format`（`json|markdown|zip`）
  - `tenant_uuid`
  - `session_id`
  - `message_id`
  - `run_id`
  - `trace_id`
  - `generated_by`
  - `generated_at`
  - `summary`
  - `timeline`
  - `nodes`
  - `errors`
  - `artifact_refs`
- **规则**:
  - 首版必须支持 Message 级 `json` 与 `markdown`。
  - Session 级与 `zip` 下载作为扩展能力预留。
  - 报告下载接口必须 root-only。

## AgentTraceSinkConfig
- **标识**: 环境配置项
- **核心字段**:
  - `enabled`
  - `local_enabled`
  - `local_dir`
  - `loki_enabled`
  - `loki_endpoint`
  - `artifact_policy`
  - `max_artifact_bytes`
  - `retention_days`
- **规则**:
  - 本地开发默认启用 local sink。
  - 生产环境可启用 loki sink。
  - local 与 loki 使用同一 `AgentTraceEvent` 模型。

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
