# SCN-180：入职/培训资料 / 学习路径

> 默认推荐：**P1 通用推荐**（学习路径需要总结归纳，但也要引用定位）。

## 典型问题

- “新同学第一周应该学什么？有没有学习路径与材料链接？”
- “XX 业务流程的关键概念是什么？有哪些常见坑？”
- “把培训课件里的核心要点总结成 checklist（带引用）。”

## 选择建议（L1/L2）

1. 场景（L1）：`入职/培训资料 / 学习路径`
2. 策略包（L2）：默认 `P1`；低成本快速验证可用 `P0`；强证据/合规要求可用 `P2`

## 建库策略（Ingestion/Index）

- 典型内容结构：章节化课件/手册 + 学习路径（需要总结归纳与引用定位）。
- 建议索引：`dense + sparse + hier`

## 在线 RAG 策略（L3，来自 rag.md）

默认会启用的模块（系统实际组合见总表）：
- `J_hier`, `H_fusion`, `F_rerank`, `C_context_enriched`, `L_feedback`

参考：
- 映射总表：`docs/guides/knowledge_space/scenary/MAP_scene_bundle_rag_profiles.md`
- 模块定义：`docs/plan/AI_engineering/knowledge/rag.md#5.2`

## 分割策略（Segment Defaults）

> 培训/手册通常章节结构明显，建议按结构切分并结合层次索引；可在入库向导第 3 步覆盖。

- 推荐分段模式：`heading`

| 策略包 | segmentMode | chunkSize | overlap | 说明 |
| --- | --- | ---:| ---:| --- |
| P0 | `heading` | 1000 | 0 | 快速验收 |
| P1 | `heading` | 800 | 140 | 默认（更强上下文） |
| P2 | `heading` | 650 | 180 | 更细粒度，利于引用与纠错 |
| P3 | `heading` | 700 | 200 | 若叠加 KG/关系约束，适度重叠 |

## 验收检查点

- 能生成“学习路径/清单”并引用到课程章节或手册段落。
- 对概念类问题能避免幻觉，无法确认时会提示缺失材料或建议补充数据源。
