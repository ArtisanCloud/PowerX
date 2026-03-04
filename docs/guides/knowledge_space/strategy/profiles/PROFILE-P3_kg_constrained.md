# PROFILE-P3：KG 约束（关系驱动 / Knowledge Graph）

> 你要用 Knowledge Graph（知识图谱）做 RAG，就选 **P3**（前提是你真的有 KG 索引与可追溯 provenance）。

## 对应策略模块（来自 rag.md）

- 默认模块集合（见 `docs/guides/knowledge_space/strategy/MAP_scene_bundle_rag_profiles.md`）：`K_kg`, `H_fusion`, `C_context_enriched`, `L_feedback`
- 模块定义来源：`docs/plan/AI_engineering/knowledge/rag.md#5.2`

## 你会得到什么

- **关系驱动召回/约束**：用实体/关系做 recall/filter，把“依赖链/拓扑/约束条件”问题做对。
- **更强可解释性**：能给出“实体 → 关系 → 结论”的链路，并引用到来源（provenance）。
- **dense 作为补充**：dense 更像“摘要/说明通道”，不是主召回通道。

## 前置依赖（必须/建议）

- 必须：`index.kg` + `index.sparse` + `index.dense`
- 必须：KG provenance（实体/关系能回溯到 chunk/source），否则“图上结论”不可验收
- 建议：稳定的实体 ID/命名规范（否则同名实体会污染图）

## 什么时候选 P3

- “依赖/引用/影响范围/上游下游/调用链”类问题：
  - SQL/配置/依赖关系库：`SCN-050`（最典型）
  - 产品兼容/配件关系：`SCN-071`（可选 KG 强化）
  - 数据字典的表关系/血缘：`SCN-101`（可选）
- 你希望答案带“关系链路解释”，而不是只给一段原文。

## 什么时候不要选 P3

- 你的知识库没有 KG 索引（只是一堆 PDF/Word）：先用 P1/P2。
- 你的问题主要是“条款事实精确与合规口径”：优先 P2。

## 验收与测试（必须要过）

- 能输出关系链路（至少 3 跳内可解释），并能引用定位到来源（chunk/source）。
- KG 缺失时必须**显式失败**（例如 `kg_required`/`kg_index_missing`），不能悄悄降级成“猜测式回答”。
- 反例测试：同名实体/错别名不应导致关系串线（需要命名/ID 规范与去重策略）。
