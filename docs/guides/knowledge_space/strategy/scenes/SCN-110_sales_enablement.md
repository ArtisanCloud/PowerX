# SCN-110：销售材料 / 话术与竞品

> 默认推荐：**H_fusion（Profile P1）**（解释归纳为主，但需要可追溯引用）。

## 典型问题

- “我们产品相对竞品 A 的优势/劣势是什么？”
- “这个行业客户最常问的点怎么回答？有没有话术模板？”
- “给我一段 30 秒/2 分钟的产品介绍（带关键证据引用）。”

## 策略包选择（A0–O）

- 推荐策略包：`H_fusion`（Profile `P1`，归纳 + 引用）
- 低成本快速验证：`A_simple`（Profile `P0`）
- 强口径/强证据：`O_crag`（Profile `P2`）

## 建库策略（Ingestion/Index）

- 典型内容结构：话术/案例/竞品对比（需要“总结归纳 + 引用定位”）。
- 建议索引：`dense + sparse`

## 在线 RAG 策略（来自 rag.md）

默认会启用的模块（系统实际组合见总表）：
- `H_fusion`, `F_rerank`, `C_context_enriched`, `L_feedback`

参考：
- 映射总表：`docs/guides/knowledge_space/strategy/MAP_scene_bundle_rag_profiles.md`
- 模块定义：`docs/plan/ai_engineering/knowledge/rag.md#5.2`

## 分割策略（Segment Defaults）

> 销售材料以“话术/要点/竞品对比”结构为主，建议按结构切分并保留上下文；可在入库向导第 3 步覆盖。

- 推荐分段模式：`heading`

| Profile | segmentMode | chunkSize | overlap | 说明 |
| --- | --- | ---:| ---:| --- |
| P0 | `heading` | 1000 | 0 | 快速验收 |
| P1 | `heading` | 800 | 120 | 默认（默认） |
| P2 | `heading` | 650 | 160 | 强口径/强证据时更细粒度 |
| P3 | `heading` | 700 | 180 | 若叠加 KG（关系/竞品映射），适度重叠 |

## 验收检查点

- 归纳类回答能给出“要点 + 证据引用”，并能定位到原文段落/页面。
- 竞品对比能避免“编造参数”，无法确认时会明确提示缺失信息。
