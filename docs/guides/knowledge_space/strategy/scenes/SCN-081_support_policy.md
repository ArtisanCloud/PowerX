# SCN-081：客服 / 售后政策与规则

> 默认推荐：**O_crag（Profile P2）**（政策答复属于高风险）。

## 典型问题

- “退款/退货规则是什么？有哪些例外？”
- “保修期如何计算？需要哪些材料？”

## 策略包选择（A0–O）

- 推荐策略包：`O_crag`（Profile `P2`，强纠错与证据一致性）
- 成本优先：`H_fusion`（Profile `P1`）

## 建库策略（Ingestion/Index）

- 典型内容结构：规则/条款/例外（高风险口径）。
- 建议索引：`dense + sparse`

## 在线 RAG 策略（来自 rag.md）

默认会启用的模块（系统实际组合见总表）：
- `H_fusion`, `O_crag`, `F_rerank`, `L_feedback`

参考：
- 映射总表：`docs/guides/knowledge_space/strategy/MAP_scene_bundle_rag_profiles.md`
- 模块定义：`docs/plan/ai_engineering/knowledge/rag.md#5.2`

## 分割策略（Segment Defaults）

> 售后政策属于“规则/例外/口径”高风险内容，优先按条款/编号切分，确保引用可追溯；可在入库向导第 3 步覆盖。

- 推荐分段模式：`clause`

| Profile | segmentMode | chunkSize | overlap | 说明 |
| --- | --- | ---:| ---:| --- |
| P0 | `clause` | 900 | 0 | 快速验收 |
| P1 | `clause` | 800 | 120 | 通用 |
| P2 | `clause` | 600 | 150 | 高风险默认（默认） |
| P3 | `clause` | 650 | 180 | 若叠加 KG/约束，适度重叠 |

## 验收检查点

- 回答必须引用条款来源；证据不足必须拒答或提示补充。
