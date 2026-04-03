# SCN-160：项目方案 / 交付文档

> 默认推荐：**H_fusion（Profile P1）**（章节结构明显，适合层次索引 + 引用定位）。

## 典型问题

- “这个项目的范围/里程碑/交付物是什么？在哪里写的？”
- “需求变更对架构/接口有什么影响？”
- “把方案中的风险与应对措施汇总一下（带引用）。”

## 策略包选择（A0–O）

- 推荐策略包：`H_fusion`（Profile `P1`，结构化文档的通用选择）
- 低成本快速验证：`A_simple`（Profile `P0`）
- 高风险口径/强证据要求：`O_crag`（Profile `P2`）

## 建库策略（Ingestion/Index）

- 典型内容结构：章节化方案/需求/设计/交付文档。
- 建议索引：`dense + sparse + hier`

## 在线 RAG 策略（来自 rag.md）

默认会启用的模块（系统实际组合见总表）：
- `J_hier`, `H_fusion`, `F_rerank`, `C_context_enriched`, `L_feedback`

参考：
- 映射总表：`docs/guides/knowledge_space/strategy/MAP_scene_bundle_rag_profiles.md`
- 模块定义：`docs/plan/ai_engineering/knowledge/rag.md#5.2`

## 分割策略（Segment Defaults）

> 项目方案/交付文档章节结构明显，建议按结构切分并结合层次索引；可在入库向导第 3 步覆盖。

- 推荐分段模式：`heading`

| Profile | segmentMode | chunkSize | overlap | 说明 |
| --- | --- | ---:| ---:| --- |
| P0 | `heading` | 1000 | 0 | 快速验收 |
| P1 | `heading` | 800 | 140 | 默认（更强上下文） |
| P2 | `heading` | 650 | 180 | 更细粒度，利于引用与纠错 |
| P3 | `heading` | 700 | 200 | 若叠加 KG/依赖关系，适度重叠 |

## 验收检查点

- 对“在哪里写的”问题能稳定定位到章节/标题（hier/section summary 可选能力）。
- 汇总类问题能给出要点归纳并引用关键章节，避免“只总结不引用”。
