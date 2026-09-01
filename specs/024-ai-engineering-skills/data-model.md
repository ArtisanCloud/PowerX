# Data Model — PowerX Skills 管理与治理

## AgentTeamOrchestration

- **归属**: `agent_teams.orchestration_spec`（JSONB），由 `agent_teams.uuid` 标识所属团队。
- **版本**: 当前唯一支持 `powerx.agent.team-orchestration/v1`。
- **作用**: 是团队任务图的唯一权威来源；Runtime 不得根据 `team_key`、显示名称、固有 Agent Key 或 Demo 名称构造任务图。
- **结构**:

```json
{
  "schema": "powerx.agent.team-orchestration/v1",
  "tasks": [
    {
      "task_id": "source_analysis",
      "node_kind": "agent_handoff",
      "assignee_role": "retriever",
      "skill_id": "marketing.audio_or_document_parse",
      "stage": 1,
      "depends_on": [],
      "failure_policy": "fail-fast"
    },
    {
      "task_id": "synthesis",
      "node_kind": "skill",
      "assignee_role": "planner",
      "skill_id": "marketing.review_summarize",
      "stage": 2,
      "depends_on": ["source_analysis"],
      "failure_policy": "fail-fast"
    }
  ]
}
```

- **任务规则**:
  - `task_id` 在团队内唯一；依赖必须指向已有任务，且整个图必须无环。
  - `node_kind=agent_handoff` 只能分配给 `retriever|executor|reviewer`；运行时按成员角色找到唯一启用的子 Agent，并验证该 Agent 绑定了 `skill_id`。
  - `node_kind=skill` 只能分配给 `planner`；运行时验证团队主 Agent 绑定了 `skill_id`。
  - `stage` 仅决定可视化和调度批次；真实执行先后以 `depends_on` 为准。
  - 依赖任务的输出以 `upstream_<task_id>` 注入下游参数，禁止通过自然语言猜测或复制整个会话历史。
- **状态规则**:
  - 新建但没有编排图的团队必须为 `disabled`，不能被选择为可执行团队。
  - 仅含有效编排图的团队可切换至 `active`；成员、技能绑定或编排图不匹配时，本轮明确失败，不得退回为单 Agent `normal_chat`。

## SkillPackageSource

- **表与标识**: `skills_package_sources.uuid`。
- **作用**: 记录外部导入包或主智能体生成 Draft 包的不可变对象存储来源；包字节不写入 PostgreSQL。
- **核心字段**:
  - `tenant_uuid`、`source_kind`（当前 `external_import|agent_authoring`）
  - `artifact_uri`（只允许 PowerX Media Storage 的 `local://`、`s3://` 或 `minio://`）、`checksum`（`sha256:`）和 `content_type`
  - `source_url`、`source_ref`（仅审计追溯）
  - `parser_version`、`standard_manifest_json`、`powerx_extension_json`
  - `created_by_member_uuid`
- **规则**:
  - 导入任务在临时目录完成解压或 clone，再把原始包冻结到当前配置的 Media Storage driver；运行时不得依赖远程仓库、OS 本地路径或 `file://` URI。默认 `local` driver 使用受控的 `local://` 逻辑 URI，并非直接文件路径。
  - `standard_manifest_json` 至少含标准 `SKILL.md` 的 `name`、`description`；可选 `powerx/` 扩展另存为 `powerx_extension_json`。

## SkillDefinitionDraft

- **表与标识**: `skills_definition_drafts.uuid`；唯一键为 `tenant_uuid + skill_id`。
- **作用**: 一个租户可编辑的 Skill 定义头，`current_revision_uuid` 是唯一当前修订指针。
- **核心字段**:
  - `tenant_uuid`、`skill_id`、`display_name_i18n`、`description_i18n`
  - `source_kind`、`package_source_uuid`（`external_import` 与 `agent_authoring` 均必填）
  - `status`（`draft|ready_for_review|instruction_only|rejected|published`）
  - `current_revision_uuid`、`created_by_member_uuid`、`updated_by_member_uuid`
- **规则**:
  - 主智能体只能调用 `skill_definition` 创作服务；其结构化 Draft 原件必须先冻结为对象存储包并创建 `agent_authoring` 来源，不能直接写 Registry 或绕过修订记录。
  - 仅标准 `SKILL.md` 的导入包必须是 `instruction_only`；补全 PowerX 执行合同前不得绑定为可执行 Team Skill。

## SkillDefinitionRevision

- **表与标识**: `skills_definition_revisions.uuid`；唯一键为 `draft_uuid + revision_number`。
- **作用**: 不可变的结构化定义快照；Definition Schema 固定为 `powerx.skill-definition/v2`。
- **核心字段**:
  - `tenant_uuid`、`draft_uuid`、`revision_number`、`definition_json`
  - `change_summary`、`source_message_uuid`、`authored_by_member_uuid`
  - `status`（`draft|published|superseded`）、`published_artifact_uri`、`published_checksum`、`published_at`
- **规则**:
  - 每次主智能体或人工修改都追加 revision，不能原地覆盖已发布修订。
  - 发布必须提供由当前修订生成的对象存储包和 `sha256:` 校验值；禁止把运行时指向本地目录、Git URL 或可变 Draft。

## SkillExecutorDeclaration

- **归属**: `SkillDefinitionRevision.definition_json.executor`。
- **版本**: `powerx.skill-definition/v2`。
- **核心字段**:
  - `type` (`llm_prompt|capability|workflow|instruction_only`)
  - `prompt_template_i18n`（仅 `llm_prompt`，按调用 locale 精确选择）、`capability_id`（仅 `capability`）、`workflow_uuid`（仅 `workflow`）
- **规则**:
  - Runtime 只能按 `executor.type` 分派，禁止按 `skill_id`、`team_key`、Agent Key 或显示名分支。
  - 每个可执行定义必须有明确输入/输出合同；`instruction_only` 不可作为 Team 执行节点。
  - `capability` 必须引用已发布且授权给当前 Agent 的 capability；禁止 Core 为示例或客户 Skill 注册业务专用 invoker。

## SkillRegistryRecord

- **现状定位**: 旧导入和平台目录的 Registry 记录；不再是用户创作的可变来源。
- **迁移目标**: Runtime 将只解析已发布的 `SkillDefinitionRevision`，Registry 保留为目录/安装索引，直至其完全收敛为发布 revision 的投影。
- **规则**: 新建或更新用户 Skill 必须走 `SkillPackageSource → SkillDefinitionDraft → SkillDefinitionRevision`，不得直接把本地或远程包写成可运行 Registry 记录。

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

## AgentResponseEnvelope

- **标识**: `schema = powerx.agent.response/v1`
- **作用**: 承载用户可见业务结果的结构化事实，供 Core 校验、SSE/历史持久化、Trace 和 Web Admin 的统一 Markdown Preview 使用。
- **核心字段**:
  - `schema`：固定 `powerx.agent.response/v1`
  - `kind`：`answer|execution_result|review_result|multi_agent_summary`
  - `outcome`：`completed|needs_action|blocked|failed`
  - `summary`：本轮可读结论
  - `answer`：对当前用户消息的直接回答
  - `acceptance[]`：每项包含 `name/status/detail`
  - `evidence[]`：可核验事实、artifact 或受控 trace 引用
  - `gaps[]`：尚不能确认的事实或阻塞原因
  - `next_actions[]`：用户或操作人员下一步
  - `artifacts[]`：可选的结果链接或下载引用
- **规则**:
  - `answer` 必填；执行、审核、发布和多智能体汇总还必须有非空 `acceptance[]`。
  - `outcome=completed` 至少需要一项可核验 `evidence[]`，否则 envelope 无效。
  - 平台显示文案、Markdown 标题和段落顺序不进入 envelope；由前端依据当前 locale 统一渲染。
  - 该对象必须同时保存在 final SSE payload、assistant message meta、AgentRunState 历史快照和 Trace final node 摘要中。
  - 不合法对象必须产生 `agent.response_contract_invalid`；禁止将其降级为任意 string 或原始 Markdown。

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
  - `response_envelope`
- **规则**:
  - 去重判断必须基于 `response_mode/capability_ids` 等 meta，不得依赖自然语言文本匹配。
  - `capability_ids[]` 只能包含当前 Agent 可见能力。
  - 不得保存完整 executor payload、原始 prompt 或敏感 context；这些内容必须进入受控 artifact。
  - `response_envelope` 保存经校验和脱敏后的结构化最终答复，用于刷新后重建统一 Preview。

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
- **标识**: `uuid`
- **核心字段**:
  - `tenant_uuid`
  - `parent_agent_uuid`
  - `team_key`（租户内稳定机器标识；不可作为显示名称）
  - `display_name_i18n`（必须含 `zh-CN`、`en-US`、`ja`、`ko`）
  - `dispatch_mode`（`serial|parallel|mixed`）
  - `default_failure_policy`（`fail-fast|continue|retry-once`）
  - `status`（`active|disabled`）
  - `created_by`, `created_at`, `updated_at`
- **规则**:
  - `uuid` 是跨域/API/审计唯一身份；内部数值主键不得出现在外部契约。
  - `team_key` 在同一租户内唯一，创建后不可由运行时解释为业务名称。
  - 当前语言缺少名称翻译时应提示配置缺失，不得显示 `team_key` 作为名称。

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
