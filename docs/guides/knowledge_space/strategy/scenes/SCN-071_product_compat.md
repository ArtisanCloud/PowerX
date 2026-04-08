# SCN-071：产品库 / 兼容性与配件关系

> 默认推荐：**K_kg（Profile P3）**；若关系数据不完整可选 **O_crag（P2）**。

## 典型问题

- “A 是否兼容 B？兼容条件是什么？”
- “A 支持哪些配件/模块？限制条件是什么？”

## 策略包选择（A0–O）

- 适用场景：`产品库 / 兼容性与配件关系`
- 推荐策略包：`K_kg`（Profile P3，关系/依赖约束）
- 若关系数据不完整：`O_crag`（Profile P2）

## 建库策略（Ingestion/Index）

- 典型内容结构：兼容矩阵/配件树/约束条件（关系信息很重要）。
- 建议索引：`dense + sparse + structured_fields`（若要 KG，需额外准备 `kg`）

## 在线 RAG 策略（来自 rag.md）

默认会启用的模块（系统实际组合见总表）：
- `H_fusion`, `O_crag`, `F_rerank`, `C_context_enriched`, `L_feedback`

参考：
- 映射总表：`docs/guides/knowledge_space/strategy/MAP_scene_bundle_rag_profiles.md`
- 模块定义：`docs/plan/ai_engineering/knowledge/rag.md#5.2`

## 分割策略（Segment Defaults）

> 兼容性/配件关系需要“条件/限制/例外”更细粒度的可追溯证据；可在入库向导第 3 步覆盖。

- 推荐分段模式：`heading`（说明文/规则条目优先按结构切分；兼容矩阵建议配合行级结构化）

| Profile | segmentMode | chunkSize | overlap | 说明 |
| --- | --- | ---:| ---:| --- |
| P0 | `heading` | 1000 | 0 | 快速验收 |
| P1 | `heading` | 800 | 120 | 通用 |
| P2 | `heading` | 650 | 160 | 更细粒度（默认） |
| P3 | `heading` | 700 | 180 | 若启用 KG 约束，适度重叠以保上下文 |

## 验收检查点

- 能返回清晰的“兼容/不兼容”结论，并引用到规则来源；复杂关系场景应能解释“为什么”。
