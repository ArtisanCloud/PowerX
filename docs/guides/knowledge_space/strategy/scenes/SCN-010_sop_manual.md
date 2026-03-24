# SCN-010：SOP/制度/产品说明（Markdown/Word 为主）

> 默认推荐：**H_fusion（Profile P1）**（必要时再升级到 `O_crag（P2）`）。

## 适用范围

- 企业内部 SOP、制度、产品说明、FAQ、操作手册（结构清晰、以标题层级为主）。

## 前置条件（建议）

- 已创建目标空间（空间只是容器，不等于一份文档）。
- 索引能力：`dense` 必须；建议启用 `sparse`；可选启用 `hier`（长文更稳）。

## 建库策略（Ingestion/Index）

- 典型内容结构：标题层级清晰、步骤/条款段落化。
- 建议索引：`dense + sparse + hier`（对应 `rag.md` 的多粒度召回）。

## 在线 RAG 策略（来自 rag.md）

默认会启用的模块（系统实际组合见总表）：
- `H_fusion`, `F_rerank`, `C_context_enriched`, `J_hier`, `L_feedback`

参考：
- 映射总表：`docs/guides/knowledge_space/strategy/MAP_scene_bundle_rag_profiles.md`
- 模块定义：`docs/plan/ai_engineering/knowledge/rag.md#5.2`

## 分割策略（Segment Defaults）

> 可在入库向导第 3 步覆盖。SOP/制度/产品说明的关键在“保留标题层级与段落边界”，便于引用与上下文扩展。

- 推荐分段模式：`heading`

| Profile | segmentMode | chunkSize | overlap | 说明 |
| --- | --- | ---:| ---:| --- |
| P0 | `heading` | 1000 | 0 | 快速验收 |
| P1 | `heading` | 800 | 120 | 通用推荐（默认） |
| P2 | `heading` | 650 | 160 | 更细粒度，提升定位与引用 |
| P3 | `heading` | 700 | 180 | 若叠加 KG/层次索引，适度重叠 |

## UI 操作步骤（推荐路径）

1. 进入「知识空间」→ 打开目标空间的「策略/配置」入口
2. 选择策略包：`H_fusion（Profile P1）`
3. 导入 1 份小样本文档（Word/Markdown，建议 3–10 页或 1k–10k 字）
4. 运行 Corpus Check（如 UI 有推荐卡片，优先一键应用推荐）
5. 进入 Playground：用 3 个问题覆盖 “步骤/定义/边界条件”

## 验收检查点

- Playground 能返回引用（引用锚点可定位到章节/段落更佳）。
- 若出现“上下文不足/答非所问”：优先启用 `sparse` 或打开“上下文增强/邻居块”能力（见策略包说明）。

## 常见问题

- SOP 很长：优先启用 `hier`（多粒度召回）再考虑提高 topK/压缩预算。
