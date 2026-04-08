# SCN-072：产品库 / 选型对比与推荐理由

> 默认推荐：**H_fusion（Profile P1）**（解释归纳为主，但仍要求可追溯引用）。

## 典型问题

- “A 和 B 的差异是什么？各自适合什么场景？”
- “在需求 X 下推荐哪个？理由是什么？”

## 策略包选择（A0–O）

- 推荐策略包：`H_fusion`（Profile `P1`，对比与归纳）
- 低成本验证：`A_simple`（Profile `P0`）
- 高风险决策：`O_crag`（Profile `P2`）

## 建库策略（Ingestion/Index）

- 典型内容结构：对比表/案例/FAQ/说明文混合，强调“理由”与“证据引用”。
- 建议索引：`dense + sparse`

## 在线 RAG 策略（来自 rag.md）

默认会启用的模块（系统实际组合见总表）：
- `H_fusion`, `F_rerank`, `C_context_enriched`, `L_feedback`

参考：
- 映射总表：`docs/guides/knowledge_space/strategy/MAP_scene_bundle_rag_profiles.md`
- 模块定义：`docs/plan/ai_engineering/knowledge/rag.md#5.2`

## 分割策略（Segment Defaults）

> 选型/对比类问题强调“对比维度与证据块”，建议按结构切分并保留上下文（可在入库向导第 3 步覆盖）。

- 推荐分段模式：`heading`

| Profile | segmentMode | chunkSize | overlap | 说明 |
| --- | --- | ---:| ---:| --- |
| P0 | `heading` | 1000 | 0 | 快速验收 |
| P1 | `heading` | 800 | 120 | 通用默认（默认） |
| P2 | `heading` | 650 | 160 | 更强证据定位/纠错 |
| P3 | `heading` | 700 | 180 | 若叠加 KG/关系约束，适度重叠 |

## 验收检查点

- 推荐必须有证据支撑（引用）；对于不确定结论要提示风险与前提。
