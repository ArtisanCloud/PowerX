# SCN-082：客服 / 故障现象与排查

> 默认推荐：**P1 通用推荐**（步骤链路 + 上下文补齐）。

## 典型问题

- “出现错误码 XXX 怎么处理？”
- “用户反馈现象 Y，可能原因有哪些？排查步骤是什么？”

## 选择建议（L1/L2）

- 场景（L1）：`客服 / 故障现象与排查`
- 策略包（L2）：默认 `P1`；高风险操作可选 `P2`

## 建库策略（Ingestion/Index）

- 典型内容结构：现象→原因→步骤（章节/步骤链路很重要）。
- 建议索引：`dense + sparse + hier`

## 在线 RAG 策略（L3，来自 rag.md）

默认会启用的模块（系统实际组合见总表）：
- `J_hier`, `H_fusion`, `F_rerank`, `C_context_enriched`, `L_feedback`

参考：
- 映射总表：`docs/guides/knowledge_space/scenary/MAP_scene_bundle_rag_profiles.md`
- 模块定义：`docs/plan/AI_engineering/knowledge/rag.md#5.2`

## 分割策略（Segment Defaults）

> 故障排查强调“步骤链路与上下文”，建议按结构切分并保留相邻步骤的重叠；可在入库向导第 3 步覆盖。

- 推荐分段模式：`heading`

| 策略包 | segmentMode | chunkSize | overlap | 说明 |
| --- | --- | ---:| ---:| --- |
| P0 | `heading` | 1000 | 0 | 快速验收 |
| P1 | `heading` | 800 | 140 | 默认（步骤相邻保留更多上下文） |
| P2 | `heading` | 650 | 180 | 高风险操作时更细粒度与更强上下文 |
| P3 | `heading` | 700 | 200 | 若叠加 KG/层次索引，适度重叠 |

## 验收检查点

- 能按“现象→原因→步骤”给出可执行建议，并引用到步骤/FAQ 来源。
