# Knowledge Space 策略包使用指南（总览 + 映射）

本目录用于存放并指导使用「**策略包（A0–O）→ 场景映射 → 模块组合**」的新模型。

如果你只记住一句话：
- **UI 只选策略包（A0–O）**，场景只是“适用范围说明”，不是必选项。

方案来源（设计与策略定义）：
- `docs/plan/ai_engineering/knowledge/knowledage_base.md`
- `docs/plan/ai_engineering/knowledge/rag.md`
- `docs/plan/ai_engineering/knowledge/rag_scene_strategy_mode.md`

---

## 使用方式（推荐）

1) 在 Web Admin 的入库向导里完成单层选择：
- **策略包（A0–O）**

2) 如果你想知道“系统到底启用了哪些 RAG 策略”，看本文的：
- “策略模块速查（来自 rag.md）”
- “策略包 → 场景映射 → 依赖索引”

3) 如果你想按具体场景一步步操作与验收：
- 直接打开对应的 `SCN-xxx` 文档（每份都写了建库策略 + 在线模块 + 验收点）。

---

## 测试影响的数据库表（按流程）

- 创建空间：
  - `powerx.knowledge_spaces`
- 绑定/激活向量索引（按维度建表）：
  - `powerx.knowledge_vector_indexes`
  - `powerx.knowledge_vectors_v1_<dim>`（动态分表，例如 `knowledge_vectors_v1_1536`）
- 入库作业：
  - `powerx.knowledge_ingestion_jobs`
  - `powerx.knowledge_chunks`
  - `powerx.knowledge_chunk_links`
  - `powerx.artifact_bundles`

---

## 模型定义（策略包优先）

- **策略包（A0–O）**  
  UI 的唯一主选项；决定在线策略方向 + 分割偏置。
- **场景（Scene）**  
  仅用于“适用范围/映射说明”，不再是 UI 必选项。
- **策略模块（Modules）**  
  来自 `docs/plan/ai_engineering/knowledge/rag.md#5.2` 的 A–O 策略清单，是实际在线管线能力（例如 `H_fusion`/`O_crag`/`K_kg`）。

SSOT（以代码为准）：
- 后端映射：`backend/config/knowledge/scene_strategy_catalog.yaml`（`strategy_packages` / `scenes`）

---

## 策略模块速查（来自 rag.md）

下面仅列出本期会在 `default_modules` 中出现的模块（完整定义看 `rag.md`）：

| 模块 ID | 对应 rag.md | 含义 |
| --- | --- | --- |
| `A_simple` | A | Simple RAG（简单切块 + dense 召回） |
| `A2_time_aware` | A2 | Time-aware（版本/时效） |
| `B_semantic_chunking` | B | Semantic Chunking（语义切块） |
| `C_context_enriched` | C | Context Enriched（上下文增强/邻居块） |
| `D_doc_augmentation` | D | Document Augmentation（离线增强字段） |
| `E_query_transform` | E | Query Transformation（查询改写/同义/结构化抽取） |
| `F_rerank` | F | Reranker（重排序） |
| `H_fusion` | H | Fusion（多路召回融合：dense+sparse+可选 hier/kg） |
| `J_hier` | J | Hierarchical Indices（层次索引/先粗后细） |
| `K_kg` | K | Knowledge Graph（知识图谱召回/过滤/解释） |
| `L_feedback` | L | Feedback Loop（反馈闭环） |
| `O_crag` | O | CRAG（纠错：证据不足/不一致时调整检索） |

---

## 策略包 → 场景映射与依赖（SSOT）

> 具体映射与依赖请以 `backend/config/knowledge/scene_strategy_catalog.yaml` 为准。

---

## 场景（第一层）索引（按分类）

- 快速验收
  - `docs/guides/knowledge_space/strategy/scenes/SCN-001_basic_ingestion_and_playground.md`

- 制度与手册（Docs）
  - `docs/guides/knowledge_space/strategy/scenes/SCN-010_sop_manual.md`

- 合同与合规（Compliance）
  - `docs/guides/knowledge_space/strategy/scenes/SCN-020_contract_quote.md`
  - `docs/guides/knowledge_space/strategy/scenes/SCN-130_compliance_regulation.md`

- 研究与资料（Research）
  - `docs/guides/knowledge_space/strategy/scenes/SCN-030_research_report.md`

- 台账与清单（Ledger）
  - `docs/guides/knowledge_space/strategy/scenes/SCN-040_table_ledger.md`

- 关系与依赖（KG & Dependency）
  - `docs/guides/knowledge_space/strategy/scenes/SCN-050_sql_config_kg.md`

- 自定义（Expert）
  - `docs/guides/knowledge_space/strategy/scenes/SCN-060_custom_expert.md`

- 产品与商品（Catalog）
  - `docs/guides/knowledge_space/strategy/scenes/SCN-070_product_specs.md`
  - `docs/guides/knowledge_space/strategy/scenes/SCN-071_product_compat.md`
  - `docs/guides/knowledge_space/strategy/scenes/SCN-072_product_selection.md`

- 支持与客服（Support）
  - `docs/guides/knowledge_space/strategy/scenes/SCN-080_support_faq.md`
  - `docs/guides/knowledge_space/strategy/scenes/SCN-081_support_policy.md`
  - `docs/guides/knowledge_space/strategy/scenes/SCN-082_support_troubleshooting.md`
  - `docs/guides/knowledge_space/strategy/scenes/SCN-170_ticket_conversations.md`

- 工程与运维（Engineering）
  - `docs/guides/knowledge_space/strategy/scenes/SCN-090_eng_runbook.md`
  - `docs/guides/knowledge_space/strategy/scenes/SCN-091_eng_incident.md`
  - `docs/guides/knowledge_space/strategy/scenes/SCN-092_eng_change.md`

- 数据与接口（Data & API）
  - `docs/guides/knowledge_space/strategy/scenes/SCN-100_api_reference.md`
  - `docs/guides/knowledge_space/strategy/scenes/SCN-101_data_dictionary.md`

- 销售与市场（Go-to-Market）
  - `docs/guides/knowledge_space/strategy/scenes/SCN-110_sales_enablement.md`
  - `docs/guides/knowledge_space/strategy/scenes/SCN-120_marketing_promo_rules.md`

- 计费与价格（Billing）
  - `docs/guides/knowledge_space/strategy/scenes/SCN-140_billing_pricing.md`

- 协作与项目（Collaboration）
  - `docs/guides/knowledge_space/strategy/scenes/SCN-150_meeting_minutes.md`
  - `docs/guides/knowledge_space/strategy/scenes/SCN-160_project_docs.md`

- 培训与入职（Enablement）
  - `docs/guides/knowledge_space/strategy/scenes/SCN-180_onboarding_training.md`

## Profile 预设（P0–P3）

- `docs/guides/knowledge_space/strategy/profiles/PROFILE-P0_basic.md`
- `docs/guides/knowledge_space/strategy/profiles/PROFILE-P1_general_recommended.md`
- `docs/guides/knowledge_space/strategy/profiles/PROFILE-P2_high_accuracy_compliance.md`
- `docs/guides/knowledge_space/strategy/profiles/PROFILE-P3_kg_constrained.md`
