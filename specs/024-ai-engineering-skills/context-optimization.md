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
  - `facts[]`
  - `decisions[]`
  - `open_issues[]`
  - `constraints[]`
  - `updated_at`
- 触发条件：
  - `session.max_tokens` 或 `session.max_kb` 超阈值
  - 会话超过 `summary_refresh_interval`
  - 手动触发（运维或调试）
- 摘要写回后，保留最近窗口（L3）与摘要（L2）共同组成会话记忆。

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
