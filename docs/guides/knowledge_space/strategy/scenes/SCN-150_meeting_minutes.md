# SCN-150：会议纪要 / 决议与行动项

> 默认推荐：**H_fusion（Profile P1）**（需要总结归纳，但要能引用定位到原始纪要）。

## 典型问题

- “上次评审会的决议是什么？有哪些行动项？负责人/截止时间？”
- “关于问题 X 的结论是什么？当时的依据是什么？”
- “把最近三次会议里提到的风险点汇总一下（带引用）。”

## 策略包选择（A0–O）

- 推荐策略包：`H_fusion`（Profile `P1`，归纳 + 引用定位）
- 低成本快速验证：`A_simple`（Profile `P0`）
- 强证据/纠错要求：`O_crag`（Profile `P2`）

## 建库策略（Ingestion/Index）

- 典型内容结构：章节化纪要 + 行动项（Owner/Due/状态）。
- 建议索引：`dense + sparse + hier`

## 在线 RAG 策略（来自 rag.md）

默认会启用的模块（系统实际组合见总表）：
- `J_hier`, `H_fusion`, `F_rerank`, `C_context_enriched`, `L_feedback`

参考：
- 映射总表：`docs/guides/knowledge_space/strategy/MAP_scene_bundle_rag_profiles.md`
- 模块定义：`docs/plan/AI_engineering/knowledge/rag.md#5.2`

## 分割策略（Segment Defaults）

> 会议纪要常见为“长段落 + 行动项”，建议按语义（句子）切分并结合层次索引；可在入库向导第 3 步覆盖。

- 推荐分段模式：`semantic`

| Profile | segmentMode | chunkSize | overlap | 说明 |
| --- | --- | ---:| ---:| --- |
| P0 | `semantic` | 1000 | 0 | 快速验收 |
| P1 | `semantic` | 800 | 140 | 默认（更强上下文） |
| P2 | `semantic` | 650 | 180 | 更细粒度，利于引用与纠错 |
| P3 | `semantic` | 700 | 200 | 若叠加 KG/关系约束，适度重叠 |

## 验收检查点

- 行动项能被结构化抽取（Owner/Due/状态），并提供引用定位。
- 多次会议汇总时能按时间/主题聚合，不混淆不同会议的结论。
