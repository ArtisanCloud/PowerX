# PROFILE-P0：基础（最小闭环 / 最少干预）

> 适合“先跑通流程、再逐步加策略”的起点。

## 对应策略模块（来自 rag.md）

- 默认模块集合（见 `docs/guides/knowledge_space/strategy/MAP_scene_bundle_rag_profiles.md`）：`A_simple`, `L_feedback`
- 模块定义来源：`docs/plan/AI_engineering/knowledge/rag.md#5.2`

## 你会得到什么

- 最短链路：**入库 → dense 召回 → 直接回答 + 引用**。
- 最低成本：模块最少、吞吐最好，但对“事实精确/合规/复杂推理”不做额外保护。

## 前置依赖（必须/建议）

- 必须：`index.dense`
- 建议：为避免“找不到证据”，内容尽量结构化（标题/小节/表格字段清晰）。

## 什么时候使用 P0 预设（Profile）

- Demo、PoC、低风险内部资料、快速验证数据源是否可用。
- UI 侧通常通过选择策略包 `A_simple` 自动落到 P0。

## 什么时候不要选 P0

- 合同/报价/促销规则/监管口径/计费价格等**高风险**答复。
- 强依赖关系/约束（Knowledge Graph/KG）的问题。

## 适用场景（映射说明）

- `SCN-001`（快速验收）
- `SCN-010`（SOP/制度）做低风险验证
- `SCN-150/160/180`（会议纪要/项目文档/培训）做“可用性基线”

## 验收与测试（最少要过）

- 能稳定命中正确段落并引用定位（至少 10 个问题）。
- 遇到证据不足时，能明确提示“未在资料中找到”，而不是编造。
- 建议先按 `docs/guides/knowledge_space/strategy/scenes/SCN-001_basic_ingestion_and_playground.md` 跑通端到端。
