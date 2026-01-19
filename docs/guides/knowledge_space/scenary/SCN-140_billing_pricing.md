# SCN-140：计费/价格规则 / 口径与例外

> 默认推荐：**P2 高准确/合规**（计费口径/价格规则属于事实精确 + 时效要求高）。

## 典型问题

- “这个套餐怎么计费？按量/包年包月的口径是什么？”
- “价格在 v1/v2 有什么差异？什么时候开始生效？”
- “哪些情况有例外/减免/封顶？对应规则在哪里？”

## 选择建议（L1/L2）

1. 场景（L1）：`计费/价格规则 / 口径与例外`
2. 策略包（L2）：默认 `P2`；风险较低的解释归纳可选 `P1`

## 建库策略（Ingestion/Index）

- 典型内容结构：规则/例外 + 版本差异（字段与时效都重要）。
- 建议索引：`dense + sparse + structured_fields + time_fields`

## 在线 RAG 策略（L3，来自 rag.md）

默认会启用的模块（系统实际组合见总表）：
- `H_fusion`, `A2_time_aware`, `O_crag`, `F_rerank`, `L_feedback`

参考：
- 映射总表：`docs/guides/knowledge_space/scenary/MAP_scene_bundle_rag_profiles.md`
- 模块定义：`docs/plan/AI_engineering/knowledge/rag.md#5.2`

## 分割策略（Segment Defaults）

> 计费/价格规则同样是“规则/例外/版本差异”高风险内容，优先按条款/编号切分；可在入库向导第 3 步覆盖。

- 推荐分段模式：`clause`

| 策略包 | segmentMode | chunkSize | overlap | 说明 |
| --- | --- | ---:| ---:| --- |
| P0 | `clause` | 900 | 0 | 快速验收 |
| P1 | `clause` | 800 | 120 | 通用 |
| P2 | `clause` | 600 | 150 | 默认（默认） |
| P3 | `clause` | 650 | 180 | 若叠加 KG/约束，适度重叠 |

## 验收检查点

- 回答必须引用具体规则来源，并能说明版本/生效时间（time_fields 建议必备）。
- 同一问题在不同版本下给出差异对比，并明确“当前租户/当前版本”的适用口径。
