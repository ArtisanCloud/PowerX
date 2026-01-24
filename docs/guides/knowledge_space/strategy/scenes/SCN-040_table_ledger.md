# SCN-040：台账/清单（Excel/CSV 结构化行）

> 默认推荐：**O_crag（Profile P2）**（需要强过滤与证据）。

## 适用范围

- 价格表、台账、库存清单、结构化报表（以“行/记录”为主的查询）。

## 前置条件（强建议）

- 索引能力：`sparse`（强）+ 行级 `dense`；字段索引（主键列/关键列）。
- 数据量大时：先从一个子表/小范围导入，验证字段与过滤再扩量。

## 建库策略（Ingestion/Index）

- 典型内容结构：行/记录为主（行级 embedding + 关键列字段化）。
- 建议索引：`sparse + structured_fields + dense`

## 在线 RAG 策略（来自 rag.md）

默认会启用的模块（系统实际组合见总表）：
- `H_fusion`, `F_rerank`, `O_crag`, `L_feedback`

参考：
- 映射总表：`docs/guides/knowledge_space/strategy/MAP_scene_bundle_rag_profiles.md`
- 模块定义：`docs/plan/AI_engineering/knowledge/rag.md#5.2`

## 分割策略（Segment Defaults）

> 台账/清单的检索单元应是“行/记录”，而不是长段落。可在入库向导第 3 步覆盖。

- 推荐分段模式：`table_row`（按行切分；用于字段过滤/精确定位）
- 推荐设置：`chunkSize=0`（不做窗口切分），避免把多条记录混在同一 chunk

| Profile | segmentMode | chunkSize | overlap | 说明 |
| --- | --- | ---:| ---:| --- |
| P0 | `table_row` | 0 | 0 | 快速验收 |
| P1 | `table_row` | 0 | 0 | 通用 |
| P2 | `table_row` | 0 | 0 | 合规/高准确（默认） |
| P3 | `table_row` | 0 | 0 | 若叠加 KG/关系约束，仍建议行级为主 |

## UI 操作步骤（推荐路径）

1. 选择场景：`台账/清单`
2. 选择策略包：`O_crag（Profile P2）`
3. 导入 1 个小 CSV/Excel（100–1000 行）
4. Playground 验证：
   - “某型号/某编号的记录是什么？”（精确定位）
   - “满足条件 X 的有哪些？”（过滤/聚合）

## 验收检查点

- 能稳定命中正确行（不应把相似行混在一起）。
- 若回答泛化：检查是否缺字段过滤/列语义抽取能力。
