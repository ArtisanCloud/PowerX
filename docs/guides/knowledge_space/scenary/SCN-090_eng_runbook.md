# SCN-090：工程/运维/故障排查（Runbook）

> 默认推荐：**P1 通用推荐**（步骤定位 + 上下文增强）。

## 典型问题

- “服务 X 报错 Y，排查步骤是什么？”
- “变更 Z 的回滚流程是什么？”
- “这个配置项影响哪些模块/服务？”

## 选择建议（L1/L2）

1. 场景（L1）：`工程/运维/故障排查`
2. 策略包（L2）：默认 `P1`；高风险变更用 `P2`；依赖拓扑明确时可用 `P3`

## 建库策略（Ingestion/Index）

- 典型内容结构：章节化 Runbook / 标准操作步骤。
- 建议索引：`dense + sparse + hier`

## 在线 RAG 策略（L3，来自 rag.md）

默认会启用的模块（系统实际组合见总表）：
- `J_hier`, `H_fusion`, `F_rerank`, `C_context_enriched`, `L_feedback`

参考：
- 映射总表：`docs/guides/knowledge_space/scenary/MAP_scene_bundle_rag_profiles.md`
- 模块定义：`docs/plan/AI_engineering/knowledge/rag.md#5.2`

## 分割策略（Segment Defaults）

> Runbook 的价值在“章节化步骤链路”，建议按结构切分并提高重叠以保留上下文；可在入库向导第 3 步覆盖。

- 推荐分段模式：`heading`

| 策略包 | segmentMode | chunkSize | overlap | 说明 |
| --- | --- | ---:| ---:| --- |
| P0 | `heading` | 1000 | 0 | 快速验收 |
| P1 | `heading` | 800 | 160 | 默认（上下文增强更稳） |
| P2 | `heading` | 650 | 200 | 高风险操作更细粒度 |
| P3 | `heading` | 700 | 220 | 若叠加 KG/依赖关系，适度重叠 |

## 验收检查点

- 能按步骤/章节返回上下文（不应只返回零散段落）。
- 若经常缺上下文：检查是否启用了 `hier`（层次索引）与上下文增强。
