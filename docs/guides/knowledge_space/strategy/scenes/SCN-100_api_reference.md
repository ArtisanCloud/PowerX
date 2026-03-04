# SCN-100：API/数据字典/接口文档

> 默认推荐：**O_crag（Profile P2）**（字段/参数/返回值属于事实精确型）。

## 典型问题

- “接口 X 的参数有哪些？类型/必填/默认值是什么？”
- “返回字段 Y 的含义是什么？示例是什么？”
- “v1 和 v2 有什么变化？是否弃用？”

## 策略包选择（A0–O）

- 推荐策略包：`O_crag`（Profile `P2`，事实精确与纠错）
- 解释归纳为主：`H_fusion`（Profile `P1`）
- 依赖链/调用链强：`K_kg`（Profile `P3`）

## 建库策略（Ingestion/Index）

- 典型内容结构：字段/参数/返回值/示例（事实精确 + 结构化字段）。
- 建议索引：`dense + sparse + structured_fields`

## 在线 RAG 策略（来自 rag.md）

默认会启用的模块（系统实际组合见总表）：
- `H_fusion`, `O_crag`, `E_query_transform`, `F_rerank`, `L_feedback`

参考：
- 映射总表：`docs/guides/knowledge_space/strategy/MAP_scene_bundle_rag_profiles.md`
- 模块定义：`docs/plan/AI_engineering/knowledge/rag.md#5.2`

## 分割策略（Segment Defaults）

> API 文档/接口说明通常按“接口/章节/字段列表”组织，建议按结构切分并保留上下文（可在入库向导第 3 步覆盖）。

- 推荐分段模式：`heading`

| Profile | segmentMode | chunkSize | overlap | 说明 |
| --- | --- | ---:| ---:| --- |
| P0 | `heading` | 1000 | 0 | 快速验收 |
| P1 | `heading` | 800 | 120 | 通用 |
| P2 | `heading` | 650 | 160 | 字段/参数更细粒度（默认） |
| P3 | `heading` | 700 | 180 | 若叠加 KG/依赖链路，适度重叠 |

## 验收检查点

- 参数/字段回答必须引用来源，并尽量结构化呈现（表格/列表）。
- 若出现版本混淆：建议补齐版本字段/更新时间（time-aware 可选能力）。
