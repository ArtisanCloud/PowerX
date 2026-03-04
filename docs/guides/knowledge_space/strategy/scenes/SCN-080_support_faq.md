# SCN-080：客服 FAQ / 运营知识

> 默认推荐：**H_fusion（Profile P1）**（强调命中率与可解释引用）。

## 典型问题

- “XX 功能怎么用/在哪里设置？”
- “出现错误码 XXX 怎么处理？”
- “退款/售后政策是什么？”

## 策略包选择（A0–O）

- 推荐策略包：`H_fusion`（Profile `P1`，命中率与引用并重）
- 低成本验证：`A_simple`（Profile `P0`）
- 投诉/合规高风险：`O_crag`（Profile `P2`）

## 建库策略（Ingestion/Index）

- 典型内容结构：问答对/短段落，同义表达多，噪声与重复也多。
- 建议索引：`dense + sparse`

## 在线 RAG 策略（来自 rag.md）

默认会启用的模块（系统实际组合见总表）：
- `H_fusion`, `E_query_transform`, `F_rerank`, `C_context_enriched`, `L_feedback`

参考：
- 映射总表：`docs/guides/knowledge_space/strategy/MAP_scene_bundle_rag_profiles.md`
- 模块定义：`docs/plan/AI_engineering/knowledge/rag.md#5.2`

## 分割策略（Segment Defaults）

> FAQ/运营知识以“短条目/问答对”为主，建议按结构切分并保持条目完整；可在入库向导第 3 步覆盖。

- 推荐分段模式：`heading`

| Profile | segmentMode | chunkSize | overlap | 说明 |
| --- | --- | ---:| ---:| --- |
| P0 | `heading` | 1000 | 0 | 快速验收 |
| P1 | `heading` | 800 | 120 | 通用默认（默认） |
| P2 | `heading` | 650 | 160 | 更细粒度，利于纠错与强引用 |
| P3 | `heading` | 700 | 180 | 若叠加 KG/关系约束，适度重叠 |

## 验收检查点

- 能覆盖同义表达（同一个问题不同说法仍能命中）。
- 引用能定位到 FAQ 条目或知识条款。
