# SCN-130：法规/监管政策 / 口径与约束

> 默认推荐：**O_crag（Profile P2）**（监管口径必须证据优先；建议启用 time-aware）。

## 典型问题

- “某条监管要求对我们业务意味着什么？有哪些约束？”
- “这项政策在 XX 日期后是否有变更？最新版是什么？”
- “哪些行为属于违规/不合规？对应条款在哪里？”

## 策略包选择（A0–O）

- 推荐策略包：`O_crag`（Profile `P2`，强证据与纠错）
- 关系/依赖约束强：`K_kg`（Profile `P3`）

## 建库策略（Ingestion/Index）

- 典型内容结构：条款/口径/时效（强时间敏感）。
- 建议索引：`dense + sparse + time_fields`

## 在线 RAG 策略（来自 rag.md）

默认会启用的模块（系统实际组合见总表）：
- `H_fusion`, `A2_time_aware`, `O_crag`, `F_rerank`, `L_feedback`

参考：
- 映射总表：`docs/guides/knowledge_space/strategy/MAP_scene_bundle_rag_profiles.md`
- 模块定义：`docs/plan/ai_engineering/knowledge/rag.md#5.2`

## 分割策略（Segment Defaults）

> 法规/监管政策以“条款/口径/生效时间”为核心，优先按条款/编号切分，并配合 time_fields；可在入库向导第 3 步覆盖。

- 推荐分段模式：`clause`

| Profile | segmentMode | chunkSize | overlap | 说明 |
| --- | --- | ---:| ---:| --- |
| P0 | `clause` | 900 | 0 | 快速验收 |
| P1 | `clause` | 800 | 120 | 通用 |
| P2 | `clause` | 600 | 150 | 默认（默认） |
| P3 | `clause` | 650 | 180 | 若叠加 KG/约束，适度重叠 |

## 验收检查点

- 结论必须引用条款，并给出“适用范围/生效时间/例外”。
- 对“时效性”敏感问题能清晰说明版本来源与更新时间（建议 time_fields 完备）。
