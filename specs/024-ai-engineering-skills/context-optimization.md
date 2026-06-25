# Context Optimization Design (024)

## 1. Goal

在保持 Agent 统一主路径（LLM 意图识别 + 计划编排）的前提下，降低每轮请求 Token 消耗与端到端时延，并保证可观测、可回放、可回归验证。

## 2. Scope

- Agent 主入口：`/api/v1/agents/invoke`、`/api/v1/agents/stream/sse`
- 统一上下文拼装链路（system prompt / tools-skills catalog / history / retrieval）
- 上下文预算管理（token budget + 分层裁剪）
- 会话摘要压缩策略（由“占位摘要”升级为结构化摘要）
- Prompt Cache 命中策略（多 Provider 兼容）
- 调用观测与审计增强（token/cached_tokens/trim_reason）

## 3. Non-Goals

- 不改变“LLM 意图识别优先，规则仅 `/command`”主策略
- 不引入“本地规则替代 LLM 直接回答”的新主路径
- 不修改技能治理生命周期状态机

## 4. Architecture

### 4.1 Context Layers

单次请求上下文按固定顺序拼装，确保语义稳定与缓存命中：

1. `L0` 固定前缀（高复用）：system policy + planner contract + output contract
2. `L1` 能力目录摘要：skills/toolings/workflows 的结构化清单（仅必要字段）
3. `L2` 会话记忆：结构化滚动摘要（facts/decisions/open_issues/constraints）
4. `L3` 最近消息窗口：最近 N 轮 user/assistant（可配置）
5. `L4` 检索上下文：RAG 片段（TopK + 去重 + 长度限制）
6. `L5` 当前用户输入

要求：
- 前缀层（L0-L2）内容与顺序必须稳定，禁止注入时间戳/trace_id 等高变化字段。
- 动态层（L3-L5）置于尾部，便于 Prompt Cache 前缀命中。

### 4.2 Budget Manager

在发送 LLM 前执行统一预算器：

1. 估算当前 `prompt_tokens`（provider tokenizer 或近似估算）
2. 若超预算，按优先级裁剪：
   - 先裁剪 L4（检索）冗余片段
   - 再裁剪 L3（最近消息）低价值轮次
   - 最后触发 L2 摘要刷新并替换旧历史
3. 保证 `reserved_completion_tokens` 预留输出空间
4. 记录 `trim_actions` 到调试日志与审计字段

### 4.3 Memory Strategy

由当前“拼接最近 N 条的占位摘要”升级为结构化摘要：

- 摘要 schema：
  - `schema`
  - `facts[]`
  - `decisions[]`
  - `open_issues[]`
  - `constraints[]`
  - `from_message_id`
  - `to_message_id`
  - `compressed_messages`
  - `recent_messages_kept`
  - `compression_policy`
  - `updated_at`
- 触发条件：
  - `session.max_tokens` 或 `session.max_kb` 超阈值
  - 会话超过 `summary_refresh_interval`
  - 手动触发（运维或调试）
- 摘要写回后，保留最近窗口（L3）与摘要（L2）共同组成会话记忆。

滚动压缩算法：

```text
input:
  active_summary = agent_chat_sessions.summary
  summary_records = agent_chat_context_summaries(session_id)
  all_messages = agent_chat_messages(session_id)
  recent_window = latest N messages
  compressible = all_messages - recent_window - pinned

if over_budget and compressible not empty:
  next_summary = merge(active_summary, compressible)
  insert next_summary record -> agent_chat_context_summaries
  save next_summary -> agent_chat_sessions.summary
  save active_context_summary_id -> agent_chat_sessions.meta
  mark/delete compressible messages according to policy
  context = next_summary + recent_window + current_user_input
```

标准实现必须同时使用两层存储：

1. `agent_chat_context_summaries` 保存每次 compact 的压缩记录、覆盖范围、checksum、summary JSON 与 artifact 引用。
2. `agent_chat_sessions.summary` 只保存当前 active summary 快照，供 Context Builder 快速读取。
3. `agent_chat_sessions.meta.active_context_summary_id` 指向当前 active summary 对应的压缩记录。

被 active summary 覆盖的非 pinned 旧消息可以删除，以控制业务表体积；压缩记录必须保留，供 root 调试、报告下载与问题回放。

约束：

1. pinned 消息不参与压缩删除。
2. 最近窗口必须保留原文，默认 20 条；Context Optimizer 仍可按 token budget 对 L3 做尾部裁剪。
3. summary 是结构化记忆，不保存完整 prompt、完整 tool payload 或 executor result。
4. 归并时必须把旧 summary 与新增旧消息一起生成新 summary，不能只总结新增消息后丢弃旧 summary。
5. 如果超预算但没有可压缩消息，必须 fail-fast，不能静默删除最近窗口。

### 4.4 Prompt Cache Strategy

采用“Provider 抽象 + 能力探测”模式，不绑定单一厂商：

- OpenAI: Prompt Caching / cached tokens
- Anthropic: Prompt Caching
- Gemini: Context Caching
- Self-host/vLLM: Prefix/KV Cache（按能力探测启用）

策略：
- 统一定义 `cache_mode=auto|force_off|force_on`
- `auto` 根据 provider capability 与 model capability 决定是否启用
- 缓存关键统计写入调用追踪：`cache_enabled/cache_hit/cached_tokens/cache_key_hash`

### 4.5 Observability

每次 LLM 调用必须记录：

- `prompt_tokens`
- `completion_tokens`
- `cached_tokens`（若 provider 支持）
- `latency_ms`
- `context_layers_size`（L0-L5 各层 token）
- `trim_actions` 与 `trim_reason`
- `memory_summary_used`（bool）

同时写入：
- debug trace 文件（`logs/agent_debug/...`）
- 审计聚合字段（用于后台趋势看板）

## 5. Data & Config Changes

### 5.1 Config

在统一 `log/agent` 配置体系下新增 context optimizer 配置：

```yaml
agent:
  context_optimizer:
    enabled: true
    max_prompt_tokens: 12000
    reserved_completion_tokens: 1200
    recent_messages: 8
    retrieval_top_k: 6
    cache_mode: auto   # auto|force_off|force_on
    summary_refresh_interval_sec: 900
```

### 5.2 Session/Trace Extensions

- 会话摘要字段支持结构化 JSON（兼容旧文本）
- 执行 trace 增加 context 优化字段：
  - `prompt_tokens`
  - `completion_tokens`
  - `cached_tokens`
  - `trim_actions`
  - `context_budget`
  - `cache_mode/cache_hit`

## 6. Rollout Plan

1. Phase A（观测优先）
- 先打点，不裁剪：采集真实 token/latency/cached_tokens 基线。

2. Phase B（软裁剪）
- 启用裁剪但保留宽松阈值，观察回答质量与时延变化。

3. Phase C（默认启用）
- 多租户灰度后全量启用；保留租户级回滚开关。

## 7. Validation

### 7.1 Functional

- 普通短问答与复杂编排均保持语义正确
- 无意图命中时仍走 LLM 对话路径，不得被本地规则替代

### 7.2 Performance

- 相比基线，P50 prompt_tokens 下降 >= 30%
- 相比基线，P95 latency 下降 >= 20%
- 缓存支持模型中，前缀缓存命中率 >= 60%

### 7.3 Regression

- 多轮会话（>=30轮）不出现上下文爆窗
- 审计可回放单次请求的裁剪决策与缓存命中信息

## 8. Risks

- 过度裁剪导致回答缺失上下文
- 不同 provider 的 token 统计口径不一致
- 缓存开关/参数透传不一致导致“看似启用实际未命中”

缓解：
- 引入最小上下文保护集（最近 2 轮 + 强约束 + 当前输入）
- 统计口径统一到平台侧归一化字段
- 在 debug trace 落盘原始 provider 请求摘要与响应 token 元数据

## 9. Planner Latency Optimization

### 9.1 Problem Statement

当前线上链路中，`skill/tooling` 执行耗时通常较低，但 Planner LLM 阶段存在显著延迟（常见 30s+）。主要原因是：

- Planner 输入上下文过大（全量候选 + 冗长 schema）
- 多数请求为“单技能直达”场景，但仍走重型候选文本
- 同会话近邻轮次重复构造相似候选清单，缺乏复用

### 9.2 Optimization Targets

- Planner `prompt_tokens`：P50 下降 >= 50%
- Planner `latency_ms`：P50 下降 >= 60%，P95 下降 >= 40%
- 保持功能正确性：skill/tooling 识别准确率不低于基线

### 9.3 Strategy A: Candidate Pre-Filter (Top-K)

在进入 Planner LLM 前，先做本地轻量召回与重排，仅保留 Top-K 候选：

1. 基于 query 与 `name/desc/tags/aliases` 做关键字与短语匹配
2. 按 `workflow|skill|tooling` 分区做最小配额（避免某一区独占）
3. 产出 `top_k_candidates`（默认 20~40）进入 Planner

约束：

- `/command` 仍按规则快捷路径，不进入该阶段
- 非 `/command` 一律经过 LLM，但其输入候选应是“筛后集合”
- 未入围候选不得进入 Planner prompt（禁止“筛后又全量拼接”）

### 9.4 Strategy B: Planner Prompt Slimming

对 Planner 输入做结构瘦身：

- 候选仅保留：`name`、`kind`、`source_scope`、`one-line desc`、`required/optional param keys`
- 移除重复解释文本与冗长 schema 展开
- 保留严格 JSON 输出契约与参数白名单校验

### 9.5 Strategy C: Decision Reuse Cache

针对高重复短问场景引入短 TTL 决策缓存：

- key：`tenant + agent + normalized_query + candidate_fingerprint + model`
- value：`planner decision (tool_calls)` + `safety metadata`
- TTL：建议 30~120 秒

命中条件：

- 候选指纹一致
- 会话上下文变化不超过安全阈值（例如 recent window 未发生高风险变更）

### 9.6 Strategy D: Planner Reasoning Guardrail

限制 Planner 输出长度与推理噪音：

- 强制 JSON-only 响应契约（失败仅一次轻量重试）
- 对支持模型设置低思考/短输出模式
- 解析失败时快速降级，不进入长链路重试风暴

### 9.7 Config Additions (Context Optimizer Page)

在 `settings/ai/context-optimizer` 增补 Planner 相关配置：

```yaml
agent:
  planner_optimizer:
    enabled: true
    candidate_top_k: 32
    per_kind_quota:
      workflow: 8
      skill: 16
      tooling: 16
    prompt_slim_mode: compact
    decision_cache_ttl_sec: 60
    decision_cache_enabled: true
```

### 9.8 Observability Additions

新增观测字段（debug trace + 审计）：

- `planner_candidates_before`
- `planner_candidates_after`
- `planner_prompt_tokens`
- `planner_latency_ms`
- `planner_cache_hit`
- `planner_cache_ttl_sec`
- `planner_retry_count`
- `planner_parse_fail_reason`

### 9.9 Rollout

1. Phase P1（观测-only）：只打点候选缩减率，不改变 Planner 输入  
2. Phase P2（灰度启用）：按租户启用 Top-K + prompt slim  
3. Phase P3（默认启用）：开启决策缓存并持续监控准确率回归  

### 9.10 Acceptance

- 典型请求（hello-echo / prompt-template）Planner 延迟显著下降
- trace 可见 `before/after` 候选量与 cache 命中信息
- 无功能回退：最终输出仍为 skill 实际内容透传（非“任务完成占位文案”）
