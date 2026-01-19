# SCN-030：论文/研究/长报告（长 PDF）

> 默认推荐：**P1 通用推荐**（高风险结论可升级 P2）。

## 适用范围

- 研究报告、白皮书、论文、长篇调研（长文、章节多、需要归纳总结）。

## 前置条件（建议）

- 索引能力：`dense` 必须；强烈建议启用 `hier`（章节/摘要级召回）；可选 `sparse`（提升精确定位）。

## 建库策略（Ingestion/Index）

- 典型内容结构：长文、多章节、需要“章节级上下文”与“段落级引用定位”。
- 建议索引：`dense + hier`（可选 `sparse`）

## 在线 RAG 策略（L3，来自 rag.md）

默认会启用的模块（系统实际组合见总表）：
- `B_semantic_chunking`, `J_hier`, `H_fusion`, `F_rerank`, `C_context_enriched`, `L_feedback`

参考：
- 映射总表：`docs/guides/knowledge_space/scenary/MAP_scene_bundle_rag_profiles.md`
- 模块定义：`docs/plan/AI_engineering/knowledge/rag.md#5.2`

## 分割策略（Segment Defaults）

> 论文/研究/长报告的关键是“语义边界 + 章节层级”。可在入库向导第 3 步覆盖。

- 推荐分段模式：`semantic`（按句子边界切分；再用长度窗口兜底）

| 策略包 | segmentMode | chunkSize | overlap | 说明 |
| --- | --- | ---:| ---:| --- |
| P0 | `semantic` | 1000 | 0 | 快速验收 |
| P1 | `semantic` | 800 | 120 | 通用默认（默认） |
| P2 | `semantic` | 650 | 160 | 更细粒度，利于精确引用 |
| P3 | `semantic` | 700 | 180 | 若叠加 KG/层次索引，适度重叠 |

## UI 操作步骤（推荐路径）

1. 选择场景：`论文/研究/长报告`
2. 选择策略包：`P1 通用推荐`
3. 导入 1 份报告（建议 20–80 页）
4. Playground 验证：
   - “本文的结论是什么？引用哪一段？”
   - “方法/实验设置是什么？”
   - “有哪些限制/风险？”

## 验收检查点

- 能按章节组织上下文（不应只返回零散段落）。
- 若答案太“摘要化”：提高上下文预算或启用上下文增强（策略包内项）。
