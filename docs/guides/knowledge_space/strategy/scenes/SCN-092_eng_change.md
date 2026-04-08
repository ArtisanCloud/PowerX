# SCN-092：工程 / 变更与发布

> 默认推荐：**O_crag（Profile P2）**（变更/发布是高风险，需证据优先）。

## 典型问题

- “发布 X 的变更点是什么？风险有哪些？回滚方案是什么？”
- “某配置变更的影响范围是什么？依据是什么？”

## 策略包选择（A0–O）

- 推荐策略包：`O_crag`（Profile `P2`，高风险纠错优先）
- 依赖关系驱动：`K_kg`（Profile `P3`）

## 建库策略（Ingestion/Index）

- 典型内容结构：变更单/发布说明/回滚方案（强时效，易版本冲突）。
- 建议索引：`dense + sparse + time_fields`

## 在线 RAG 策略（来自 rag.md）

默认会启用的模块（系统实际组合见总表）：
- `H_fusion`, `A2_time_aware`, `O_crag`, `F_rerank`, `L_feedback`

参考：
- 映射总表：`docs/guides/knowledge_space/strategy/MAP_scene_bundle_rag_profiles.md`
- 模块定义：`docs/plan/ai_engineering/knowledge/rag.md#5.2`

## 分割策略（Segment Defaults）

> 变更/发布文档通常有明确的“变更点/风险/回滚”结构，且对版本/时间敏感；建议按结构切分并保留足够上下文，可在入库向导第 3 步覆盖。

- 推荐分段模式：`heading`

| Profile | segmentMode | chunkSize | overlap | 说明 |
| --- | --- | ---:| ---:| --- |
| P0 | `heading` | 1000 | 0 | 快速验收 |
| P1 | `heading` | 800 | 120 | 通用 |
| P2 | `heading` | 650 | 180 | 高风险默认（默认） |
| P3 | `heading` | 700 | 200 | 若叠加 KG/依赖关系，适度重叠 |

## 验收检查点

- 变更点/风险/回滚必须引用到来源（变更单/发布说明/Runbook）。
