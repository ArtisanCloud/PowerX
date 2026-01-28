# SCN-101：数据字典 / 表结构与字段口径

> 默认推荐：**O_crag（Profile P2）**（字段口径是事实精确型）。

## 典型问题

- “字段 Y 的含义/口径是什么？如何计算？”
- “表 A 和表 B 的关系是什么？主键/外键是什么？”

## 策略包选择（A0–O）

- 推荐策略包：`O_crag`（Profile `P2`，字段口径与证据一致性）
- 解释归纳为主：`H_fusion`（Profile `P1`）

## 建库策略（Ingestion/Index）

- 典型内容结构：表/字段/口径/示例（结构化字段强相关）。
- 建议索引：`dense + sparse + structured_fields`

## 在线 RAG 策略（来自 rag.md）

默认会启用的模块（系统实际组合见总表）：
- `H_fusion`, `O_crag`, `E_query_transform`, `F_rerank`, `L_feedback`

参考：
- 映射总表：`docs/guides/knowledge_space/strategy/MAP_scene_bundle_rag_profiles.md`
- 模块定义：`docs/plan/AI_engineering/knowledge/rag.md#5.2`

## 分割策略（Segment Defaults）

> 数据字典的核心单元是“字段/表”，建议行级/条目级切分；可在入库向导第 3 步覆盖。

- 推荐分段模式：`table_row`
- 推荐设置：`chunkSize=0`（不做窗口切分），避免混淆不同字段口径

| Profile | segmentMode | chunkSize | overlap | 说明 |
| --- | --- | ---:| ---:| --- |
| P0 | `table_row` | 0 | 0 | 快速验收 |
| P1 | `table_row` | 0 | 0 | 通用 |
| P2 | `table_row` | 0 | 0 | 字段口径更稳定（默认） |
| P3 | `table_row` | 0 | 0 | 若叠加 KG（血缘/依赖），仍建议条目级为主 |

## 验收检查点

- 字段口径必须引用来源；尽量结构化返回（字段名/类型/说明/示例）。
