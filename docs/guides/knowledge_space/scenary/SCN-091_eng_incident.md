# SCN-091：工程 / 故障排查与应急响应

> 默认推荐：**P1 通用推荐**（步骤链路 + 上下文增强）；依赖拓扑明确时可选 `P3`。

## 典型问题

- “服务 X 发生故障，排查/止血/恢复步骤是什么？”
- “这次故障可能影响哪些系统？依据是什么？”

## 选择建议（L1/L2）

- 场景（L1）：`工程 / 故障排查与应急响应`
- 策略包（L2）：默认 `P1`；高风险变更/合规可选 `P2`

## 建库策略（Ingestion/Index）

- 典型内容结构：应急步骤链路 + 复盘要点（章节/步骤强相关）。
- 建议索引：`dense + sparse + hier`

## 在线 RAG 策略（L3，来自 rag.md）

默认会启用的模块（系统实际组合见总表）：
- `J_hier`, `H_fusion`, `F_rerank`, `C_context_enriched`, `L_feedback`

参考：
- 映射总表：`docs/guides/knowledge_space/scenary/MAP_scene_bundle_rag_profiles.md`
- 模块定义：`docs/plan/AI_engineering/knowledge/rag.md#5.2`

## 分割策略（Segment Defaults）

> 应急响应同样依赖“步骤链路 + 上下文”，建议按结构切分并提高重叠；可在入库向导第 3 步覆盖。

- 推荐分段模式：`heading`

| 策略包 | segmentMode | chunkSize | overlap | 说明 |
| --- | --- | ---:| ---:| --- |
| P0 | `heading` | 1000 | 0 | 快速验收 |
| P1 | `heading` | 800 | 160 | 默认 |
| P2 | `heading` | 650 | 200 | 高风险操作更细粒度 |
| P3 | `heading` | 700 | 220 | 若叠加 KG/依赖关系，适度重叠 |

## 验收检查点

- 能给出“止血→定位→恢复→复盘”的结构化步骤，并引用到 Runbook/变更记录。
