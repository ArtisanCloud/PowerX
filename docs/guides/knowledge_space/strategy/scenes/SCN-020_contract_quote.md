# SCN-020：合同/报价（PDF/Word/Excel 多）

> 默认推荐：**O_crag（Profile P2）**（可选 `K_kg（P3）` 用于实体对齐/条款约束）。

## 适用范围

- 合同条款、报价单、对外承诺文件（高风险问答：必须“强证据/强引用/冲突纠错”）。

## 前置条件（强建议）

- 索引能力：`dense + sparse`（必备组合）；可选 `time-aware`（版本/时效）；表格字段索引（报价/清单）。
- 如果 PDF 扫描占比高：需要 OCR 能力，否则会 degraded/blocked（以系统提示为准）。

## 建库策略（Ingestion/Index）

- 典型内容结构：条款/段落 + 表格（报价清单/附件）混合。
- 建议索引：`dense + sparse + structured_fields + time_fields`

## 在线 RAG 策略（来自 rag.md）

默认会启用的模块（系统实际组合见总表）：
- `H_fusion`, `A2_time_aware`, `O_crag`, `F_rerank`, `D_doc_augmentation`, `L_feedback`

参考：
- 映射总表：`docs/guides/knowledge_space/strategy/MAP_scene_bundle_rag_profiles.md`
- 模块定义：`docs/plan/ai_engineering/knowledge/rag.md#5.2`

## 分割策略（Segment Defaults）

> 合同/报价的核心是“条款/编号/附件表格”的可追溯引用。可在入库向导第 3 步覆盖。

- 推荐分段模式：`clause`（按 `1.1/1.2/第X条` 等条款边界；未命中则按段落）

| Profile | segmentMode | chunkSize | overlap | 说明 |
| --- | --- | ---:| ---:| --- |
| P0 | `clause` | 900 | 0 | 快速验收 |
| P1 | `clause` | 800 | 120 | 通用 |
| P2 | `clause` | 600 | 150 | 合规/证据更强（默认） |
| P3 | `clause` | 650 | 180 | 条款 + 关系约束（KG） |

## UI 操作步骤（推荐路径）

1. 选择场景：`合同/报价`
2. 选择策略包：`O_crag（Profile P2）`
3. 导入 1 份合同样本（或 1 份报价表）
4. 运行 Corpus Check：确认“扫描/OCR/表格占比”与推荐策略一致
5. Playground 验证：
   - “某条款的具体要求是什么？”（事实查找）
   - “是否允许 XX？依据是什么？”（必须引用）
   - “两个版本的差异是什么？”（若启用 time-aware/版本字段）

## 验收检查点

- 回答必须带引用；证据不足时应拒答/提示补充，而不是编造。
- 若误召回：检查是否启用了 `sparse`，以及是否需要字段过滤（金额/日期/编号）。
