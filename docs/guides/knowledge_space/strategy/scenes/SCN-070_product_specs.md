# SCN-070：产品库 / 规格参数查询

> 默认推荐：**O_crag（Profile P2）**（事实精确优先）。

## 典型问题

- “型号 A 的参数/尺寸/功耗是多少？”
- “A 的价格/保修期/规格口径是什么？”
- “这个参数来自哪份资料？引用依据是什么？”

## 策略包选择（A0–O）

- 适用场景：`产品库 / 规格参数查询`
- 推荐策略包：`O_crag`（Profile P2，事实与证据优先）
- 成本优先：`H_fusion`（Profile P1）

## 建库策略（Ingestion/Index）

- 典型内容结构：字段/参数密集（型号/尺寸/单位/版本），常混合数据源（DB+文档）。
- 建议索引：`dense + sparse + structured_fields`

## 在线 RAG 策略（来自 rag.md）

默认会启用的模块（系统实际组合见总表）：
- `H_fusion`, `O_crag`, `F_rerank`, `E_query_transform`, `L_feedback`

参考：
- 映射总表：`docs/guides/knowledge_space/strategy/MAP_scene_bundle_rag_profiles.md`
- 模块定义：`docs/plan/ai_engineering/knowledge/rag.md#5.2`

## 分割策略（Segment Defaults）

> 产品规格查询通常“结构化字段 + 说明文本”混合，优先按标题/段落保留结构锚点；表格部分建议行级处理（后续由 processor/profile 提供更强结构化）。

- 推荐分段模式：`heading`

| Profile | segmentMode | chunkSize | overlap | 说明 |
| --- | --- | ---:| ---:| --- |
| P0 | `heading` | 1000 | 0 | 快速验收 |
| P1 | `heading` | 800 | 120 | 通用 |
| P2 | `heading` | 650 | 160 | 规格/参数更细粒度（默认） |
| P3 | `heading` | 700 | 180 | 若叠加 KG（部件/配件关系），适度重叠 |

## 验收检查点

- 关键参数必须引用来源（页码/段落/字段），证据不足应拒答/提示补充。
