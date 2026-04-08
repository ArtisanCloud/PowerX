# SCN-170：工单/聊天记录 / 问题追踪

> 默认推荐：**H_fusion（Profile P1）**（对话多轮，需要上下文补齐与可追溯引用）。

## 典型问题

- “这个工单目前进展如何？最后一次回复说了什么？”
- “用户 X 反复反馈的问题是什么？我们给过哪些解决方案？”
- “把本周同类问题的处理结论/解决路径汇总一下（带引用）。”

## 策略包选择（A0–O）

- 推荐策略包：`H_fusion`（Profile `P1`，对话类通用）
- 低成本快速验证：`A_simple`（Profile `P0`）
- 强证据与纠错：`O_crag`（Profile `P2`）

## 建库策略（Ingestion/Index）

- 典型内容结构：多轮对话 + 时间窗/话题聚合（避免串线）。
- 建议索引：`dense + sparse`

## 在线 RAG 策略（来自 rag.md）

默认会启用的模块（系统实际组合见总表）：
- `H_fusion`, `E_query_transform`, `F_rerank`, `C_context_enriched`, `L_feedback`

参考：
- 映射总表：`docs/guides/knowledge_space/strategy/MAP_scene_bundle_rag_profiles.md`
- 模块定义：`docs/plan/ai_engineering/knowledge/rag.md#5.2`

## 分割策略（Segment Defaults）

> 对话/工单的核心边界是“轮次/发言人/时间窗”，建议按对话轮次切分，并适度重叠保留上下文；可在入库向导第 3 步覆盖。

- 推荐分段模式：`conversation`（按 `发言人: 内容` 的轮次边界；未命中则按段落）

| Profile | segmentMode | chunkSize | overlap | 说明 |
| --- | --- | ---:| ---:| --- |
| P0 | `conversation` | 1000 | 0 | 快速验收 |
| P1 | `conversation` | 800 | 160 | 默认（更强上下文） |
| P2 | `conversation` | 650 | 200 | 更强证据/纠错时更细粒度 |
| P3 | `conversation` | 700 | 220 | 若叠加 KG/关系约束，适度重叠 |

## 验收检查点

- 能按话题/时间窗聚合对话，避免不同工单串线。
- 对“最后一次回复/谁说的”能引用到具体对话片段与时间点。
