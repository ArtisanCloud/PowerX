# Context Optimizer 配置页面开发设计

## 1. 背景与目标

当前 `agent.context_optimizer.*` 与 `log.agent_debug.*` 主要依赖 `config.yaml` 手工维护。  
在真实联调中，这些参数属于高频调优项（成本、延迟、稳定性），需要支持 UI 在线调整与快速回滚。

本设计目标：

1. 将 Context Optimizer 关键参数从静态 YAML 提升为可视化配置页面。
2. 支持租户级/环境级配置覆盖，不影响全局默认。
3. 支持“立即生效 + 一键回滚 + 变更审计”。
4. 保持与现有技能/Agent 流程兼容，不改变主执行语义（仅优化上下文成本与可观测性）。

## 2. 范围

### 2.1 In Scope（本期）

1. 配置项管理（读/写/发布）：
   - `enabled`
   - `max_prompt_tokens`
   - `reserved_completion_tokens`
   - `recent_messages`
   - `retrieval_top_k`
   - `cache_mode`（`auto|force_on|force_off`）
   - `summary_refresh_interval_sec`
   - `debug_trace_enabled`（映射 `log.agent_debug.enabled`）
2. 环境维度：`env`（如 `dev/default/prod`）
3. 租户维度：`tenant_uuid`（为空表示 system 默认）
4. 后端运行时读取 DB 配置覆盖静态配置。
5. 页面提供“保存草稿 / 发布生效 / 回滚到历史版本”。

### 2.2 Out of Scope（后续）

1. 按单 Agent 覆盖 context optimizer。
2. 自动 A/B 实验与自动调参。
3. 复杂规则引擎（按消息类型动态预算）。

## 3. 总体方案

采用“静态配置兜底 + DB 动态覆盖”的双层配置模型：

1. 启动默认：读取 `config.yaml`。
2. 请求生效：优先读取 DB 中 `(env, tenant_uuid)` 的已发布版本。
3. DB 无记录时回退到静态配置。

优先级：

1. 请求级临时覆盖（如果未来支持）
2. DB 已发布租户配置
3. DB 已发布系统配置（tenant 为空）
4. `config.yaml` 默认

## 4. 数据模型设计

新增表：`agent_runtime_configs`

建议字段：

1. `id` bigint PK
2. `uuid` uuid
3. `env` varchar(32) not null
4. `tenant_uuid` varchar(36) null
5. `scope` varchar(16) not null default `tenant`（`system|tenant`）
6. `config_type` varchar(64) not null default `context_optimizer`
7. `version` int not null
8. `status` varchar(16) not null default `draft`（`draft|published|archived`）
9. `config_json` jsonb not null
10. `change_reason` text
11. `created_by` bigint
12. `published_by` bigint
13. `published_at` timestamptz
14. `created_at/updated_at/deleted_at`

唯一约束建议：

1. `(env, tenant_uuid, config_type, version)` 唯一
2. `(env, tenant_uuid, config_type)` 下仅允许 1 条 `published`

## 5. API 设计

管理端路由建议：`/api/v1/admin/agents/context-optimizer/*`

### 5.1 查询当前生效配置

`GET /api/v1/admin/agents/context-optimizer/active?env=dev`

返回：

1. `source`（`tenant|system|yaml_default`）
2. `config`
3. `version`（若来自 DB）

### 5.2 保存草稿

`POST /api/v1/admin/agents/context-optimizer/drafts`

请求体：

1. `env`
2. `tenant_uuid`（可空）
3. `config`
4. `change_reason`

### 5.3 发布配置

`POST /api/v1/admin/agents/context-optimizer/publish`

请求体：

1. `env`
2. `tenant_uuid`
3. `version`
4. `change_reason`

### 5.4 历史版本

`GET /api/v1/admin/agents/context-optimizer/versions?env=dev&page=1&page_size=20`

### 5.5 回滚

`POST /api/v1/admin/agents/context-optimizer/rollback`

请求体：

1. `env`
2. `tenant_uuid`
3. `target_version`
4. `change_reason`

## 6. 后端实现要点

### 6.1 配置读取链路

在现有 `manager/bootstrap/chat_handler` 读取 `ContextOptimizerConfig` 的位置增加 DB 覆盖：

1. 请求进入时根据 `env + tenant_uuid` 获取 active config。
2. 命中则覆盖内存中的 `ContextOptimizerConfig`。
3. 未命中走静态配置。

### 6.2 缓存策略

使用统一 PowerX Cache 封装（Redis 驱动）缓存 active config：

1. Key：`agent:ctxopt:active:{env}:{tenant_uuid|system}`
2. TTL：60s（可配置）
3. 发布/回滚时主动失效对应 key

说明：配置变更频率低于聊天请求频率，必须缓存，避免每次请求查库。

### 6.3 参数校验

发布前强校验：

1. `max_prompt_tokens`：`[1024, 200000]`
2. `reserved_completion_tokens`：`[256, 32000]`
3. `reserved_completion_tokens < max_prompt_tokens`
4. `recent_messages`：`[1, 100]`
5. `retrieval_top_k`：`[0, 50]`
6. `summary_refresh_interval_sec`：`[30, 86400]`
7. `cache_mode`：枚举校验

### 6.4 审计

写入 audit event：

1. `context_optimizer.draft.saved`
2. `context_optimizer.published`
3. `context_optimizer.rolled_back`

审计字段至少包含 `env/tenant_uuid/version/operator/change_reason`。

## 7. 前端页面设计

建议挂载在：`设置 -> AI -> Context Optimizer`

访问路径（Web Admin）：

1. 一级：`设置`（`/settings`）
2. 二级：`AI 设置`（`/settings/ai`）
3. 三级：`Context Optimizer`（`/settings/ai/context-optimizer`）

页面结构：

1. 顶部：环境切换、作用域提示（系统/租户）、当前生效版本
2. 中部：参数表单（分组）
   - Token 预算
   - 上下文窗口
   - Prompt Cache
   - 调试追踪
3. 右侧：实时校验与“预计影响”提示
4. 底部：`保存草稿`、`发布`、`查看历史`、`回滚`
5. 历史抽屉：版本 diff（JSON 对比）

交互要求：

1. 修改未发布时离开页面弹确认。
2. 发布前二次确认，显示 diff 与影响范围。
3. 回滚必须填写原因。

## 8. 权限设计

建议权限点：

1. `agent.context_optimizer.read`
2. `agent.context_optimizer.write`
3. `agent.context_optimizer.publish`
4. `agent.context_optimizer.rollback`

默认仅租户管理员和系统管理员可发布/回滚。

## 9. 可观测与验收

### 9.1 指标

1. `agent_context_optimizer_config_read_total{source=...}`
2. `agent_context_optimizer_cache_hit_total`
3. `agent_context_optimizer_publish_total`
4. `agent_context_optimizer_rollback_total`

### 9.2 验收标准

1. 页面修改并发布后，新请求 1 分钟内看到 `response.usage.cache_mode/trim_actions` 变化。
2. 回滚后行为恢复（同一测试输入，`prompt_tokens` 回到回滚前区间）。
3. 运行中无明显性能回退（P95 配置读取耗时 < 5ms，缓存命中时 < 1ms）。

## 10. 交付计划（建议）

### Phase A（后端基础）

1. 表结构 + repository + service
2. active config 读取与缓存
3. 发布/回滚 API + 审计

### Phase B（前端页面）

1. 配置页表单 + 校验
2. 历史版本列表 + diff
3. 发布/回滚交互

### Phase C（联调与回归）

1. 与 `agent_debug` 日志联调验证
2. 集成测试（配置读取优先级、回滚一致性、缓存失效）
3. 文档更新（guides + test_use_cases）

## 11. 风险与对策

1. 风险：误配导致上下文过短，回答质量下降。  
对策：前端提供安全阈值提示 + 发布二次确认 + 快速回滚。

2. 风险：频繁改配置引发行为抖动。  
对策：版本化与变更审计，限制发布权限。

3. 风险：每次请求查库导致延迟上升。  
对策：Redis 缓存 + 发布事件失效。
