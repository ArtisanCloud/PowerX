# SCN-120：市场活动 / 促销规则

> 默认推荐：**O_crag（Profile P2）**（促销口径属于高风险答复，需要强证据与纠错）。

## 典型问题

- “这次活动的优惠规则是什么？哪些情况不适用？”
- “新老客户是否同享？叠加券/满减能否一起用？”
- “活动时间/适用范围/例外条款分别是什么？”

## 策略包选择（A0–O）

- 推荐策略包：`O_crag`（Profile `P2`，强证据 + 纠错）
- 解释归纳为主且风险较低：`H_fusion`（Profile `P1`）

## 建库策略（Ingestion/Index）

- 典型内容结构：规则/例外/不适用（高风险口径）。
- 建议索引：`dense + sparse`

## 在线 RAG 策略（来自 rag.md）

默认会启用的模块（系统实际组合见总表）：
- `H_fusion`, `O_crag`, `F_rerank`, `L_feedback`

参考：
- 映射总表：`docs/guides/knowledge_space/strategy/MAP_scene_bundle_rag_profiles.md`
- 模块定义：`docs/plan/ai_engineering/knowledge/rag.md#5.2`

## 分割策略（Segment Defaults）

> 促销规则是典型“规则/例外/条件”内容，优先按条款/编号切分，确保引用可追溯；可在入库向导第 3 步覆盖。

- 推荐分段模式：`clause`

| Profile | segmentMode | chunkSize | overlap | 说明 |
| --- | --- | ---:| ---:| --- |
| P0 | `clause` | 900 | 0 | 快速验收 |
| P1 | `clause` | 800 | 120 | 通用 |
| P2 | `clause` | 600 | 150 | 默认（默认） |
| P3 | `clause` | 650 | 180 | 若叠加 KG/约束，适度重叠 |

## 验收检查点

- 规则/例外必须引用来源（条款/公告/FAQ），并尽量结构化呈现（列表/条件分支）。
- 若遇到“时间/版本冲突”，必须提示以最新公告为准，并建议补齐时间字段过滤。
