# Knowledge Space 场景/策略使用指南（总览 + 映射）

本目录用于存放并指导使用「**场景（L1）× 策略包（L2）× RAG 策略模块（L3）**」三层模型。

如果你只记住一句话：
- **L1 选“你要解决的业务问法/内容结构”**
- **L2 选“成本/护栏强度等级（P0–P3）”**
- **L3 是系统实际启用的 RAG 模块组合（来自 `rag.md`），由 L1/L2 决定**

方案来源（设计与策略定义）：
- `docs/plan/AI_engineering/knowledge/knowledage_base.md`
- `docs/plan/AI_engineering/knowledge/rag.md`
- `docs/plan/AI_engineering/knowledge/rag_scene_strategy_mode.md`

---

## 使用方式（推荐）

1) 在 Web Admin 的入库向导里完成两层选择：
- 第 1 层：业务场景（L1）
- 第 2 层：策略包（L2，P0–P3）

2) 如果你想知道“系统到底启用了哪些 RAG 策略”，看本文的：
- “L3 模块速查”
- “策略包默认模块集合”
- “场景映射总表（L1→默认 L2→默认 L3→索引前置）”

3) 如果你想按具体场景一步步操作与验收：
- 直接打开对应的 `SCN-xxx` 文档（每份都写了建库策略 + 在线模块 + 验收点）。

---

## 三层模型定义（L1/L2/L3）

- **L1：业务场景（Scene）**  
  决定默认的建库策略（解析/切块/增强/索引类型：dense/sparse/hier/kg/字段/时间等）。
- **L2：策略包（Bundle）**  
  决定在线策略的“强度/成本/护栏级别”（P0–P3）。注意：不是所有场景都允许所有策略包。
- **L3：RAG 策略模块（Modules）**  
  来自 `docs/plan/AI_engineering/knowledge/rag.md#5.2` 的 A–O 策略清单，是真正的“在线管线能力组合”（例如 `H_fusion`/`O_crag`/`K_kg`）。

SSOT（以代码为准）：
- 后端映射：`backend/config/knowledge/scene_strategy_catalog.yaml`（`default_modules` / `prerequisites.index`）

---

## L3 模块速查（来自 rag.md）

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

## 策略包（L2）默认模块集合（SSOT）

> 注意：场景（L1）可能会在此基础上叠加/替换模块（例如 SOP 默认加 `J_hier`，合同默认加 `A2_time_aware`、`D_doc_augmentation`）。

| 策略包 | 默认模块（L3） |
| --- | --- |
| P0 `p0_basic` | `A_simple`, `L_feedback` |
| P1 `p1_general` | `H_fusion`, `F_rerank`, `C_context_enriched`, `L_feedback` |
| P2 `p2_high_accuracy` | `H_fusion`, `O_crag`, `F_rerank`, `L_feedback` |
| P3 `p3_kg_strong` | `K_kg`, `H_fusion`, `C_context_enriched`, `L_feedback` |

---

## 场景映射总表（L1→默认 L2→默认 L3→索引前置）

> 这张表用于回答你最关心的三件事：  
> 1) 这个场景默认推荐哪个策略包？  
> 2) 系统实际会启用哪些 RAG 模块？  
> 3) 要把这些模块跑起来，我建库时必须具备哪些索引/资产？

| SceneKey | SCN 指导 | 默认 L2 | 默认 L3 模块（系统实际启用） | Seg 默认（mode/size/overlap） | 建库索引前置（Index） |
| --- | --- | --- | --- | --- | --- |
| `sop` | `SCN-010_sop_manual.md` | P1 | `H_fusion`,`F_rerank`,`C_context_enriched`,`J_hier`,`L_feedback` | `heading 800/120` | `dense`,`sparse`,`hier` |
| `contract_quote` | `SCN-020_contract_quote.md` | P2 | `H_fusion`,`A2_time_aware`,`O_crag`,`F_rerank`,`D_doc_augmentation`,`L_feedback` | `clause 600/150` | `dense`,`sparse`,`time_fields`,`structured_fields` |
| `research_longdoc` | `SCN-030_research_report.md` | P1 | `B_semantic_chunking`,`J_hier`,`H_fusion`,`F_rerank`,`C_context_enriched`,`L_feedback` | `semantic 800/120` | `dense`,`hier` |
| `ledger_table` | `SCN-040_table_ledger.md` | P2 | `H_fusion`,`F_rerank`,`O_crag`,`L_feedback` | `table_row 0/0` | `sparse`,`structured_fields`,`dense` |
| `sql_kg` | `SCN-050_sql_config_kg.md` | P3 | `K_kg`,`H_fusion`,`C_context_enriched`,`L_feedback` | `code_block 900/180` | `kg`,`sparse`,`dense` |
| `custom_expert` | `SCN-060_custom_expert.md` | P1 | `H_fusion`,`L_feedback` | `unit 0/0` | `dense` |
| `product_specs` | `SCN-070_product_specs.md` | P2 | `H_fusion`,`O_crag`,`F_rerank`,`E_query_transform`,`L_feedback` | `heading 650/160` | `dense`,`sparse`,`structured_fields` |
| `product_compat` | `SCN-071_product_compat.md` | P2 | `H_fusion`,`O_crag`,`F_rerank`,`C_context_enriched`,`L_feedback` | `heading 650/160` | `dense`,`sparse`,`structured_fields` |
| `product_selection` | `SCN-072_product_selection.md` | P1 | `H_fusion`,`F_rerank`,`C_context_enriched`,`L_feedback` | `heading 800/120` | `dense`,`sparse` |
| `support_faq` | `SCN-080_support_faq.md` | P1 | `H_fusion`,`E_query_transform`,`F_rerank`,`C_context_enriched`,`L_feedback` | `heading 800/120` | `dense`,`sparse` |
| `support_policy` | `SCN-081_support_policy.md` | P2 | `H_fusion`,`O_crag`,`F_rerank`,`L_feedback` | `clause 600/150` | `dense`,`sparse` |
| `support_troubleshooting` | `SCN-082_support_troubleshooting.md` | P1 | `J_hier`,`H_fusion`,`F_rerank`,`C_context_enriched`,`L_feedback` | `heading 800/140` | `dense`,`sparse`,`hier` |
| `eng_runbook` | `SCN-090_eng_runbook.md` | P1 | `J_hier`,`H_fusion`,`F_rerank`,`C_context_enriched`,`L_feedback` | `heading 800/160` | `dense`,`sparse`,`hier` |
| `eng_incident` | `SCN-091_eng_incident.md` | P1 | `J_hier`,`H_fusion`,`F_rerank`,`C_context_enriched`,`L_feedback` | `heading 800/160` | `dense`,`sparse`,`hier` |
| `eng_change` | `SCN-092_eng_change.md` | P2 | `H_fusion`,`A2_time_aware`,`O_crag`,`F_rerank`,`L_feedback` | `heading 650/180` | `dense`,`sparse`,`time_fields` |
| `api_reference` | `SCN-100_api_reference.md` | P2 | `H_fusion`,`O_crag`,`E_query_transform`,`F_rerank`,`L_feedback` | `heading 650/160` | `dense`,`sparse`,`structured_fields` |
| `data_dictionary` | `SCN-101_data_dictionary.md` | P2 | `H_fusion`,`O_crag`,`E_query_transform`,`F_rerank`,`L_feedback` | `table_row 0/0` | `dense`,`sparse`,`structured_fields` |
| `sales_enablement` | `SCN-110_sales_enablement.md` | P1 | `H_fusion`,`F_rerank`,`C_context_enriched`,`L_feedback` | `heading 800/120` | `dense`,`sparse` |
| `marketing_promo_rules` | `SCN-120_marketing_promo_rules.md` | P2 | `H_fusion`,`O_crag`,`F_rerank`,`L_feedback` | `clause 600/150` | `dense`,`sparse` |
| `compliance_regulation` | `SCN-130_compliance_regulation.md` | P2 | `H_fusion`,`A2_time_aware`,`O_crag`,`F_rerank`,`L_feedback` | `clause 600/150` | `dense`,`sparse`,`time_fields` |
| `billing_pricing` | `SCN-140_billing_pricing.md` | P2 | `H_fusion`,`A2_time_aware`,`O_crag`,`F_rerank`,`L_feedback` | `clause 600/150` | `dense`,`sparse`,`structured_fields`,`time_fields` |
| `meeting_minutes` | `SCN-150_meeting_minutes.md` | P1 | `J_hier`,`H_fusion`,`F_rerank`,`C_context_enriched`,`L_feedback` | `semantic 800/140` | `dense`,`sparse`,`hier` |
| `project_docs` | `SCN-160_project_docs.md` | P1 | `J_hier`,`H_fusion`,`F_rerank`,`C_context_enriched`,`L_feedback` | `heading 800/140` | `dense`,`sparse`,`hier` |
| `ticket_conversations` | `SCN-170_ticket_conversations.md` | P1 | `H_fusion`,`E_query_transform`,`F_rerank`,`C_context_enriched`,`L_feedback` | `conversation 800/160` | `dense`,`sparse` |
| `onboarding_training` | `SCN-180_onboarding_training.md` | P1 | `J_hier`,`H_fusion`,`F_rerank`,`C_context_enriched`,`L_feedback` | `heading 800/140` | `dense`,`sparse`,`hier` |

---

## 场景（第一层）索引（按分类）

- 快速验收
  - `docs/guides/knowledge_space/scenary/SCN-001_basic_ingestion_and_playground.md`

- 制度与手册（Docs）
  - `docs/guides/knowledge_space/scenary/SCN-010_sop_manual.md`

- 合同与合规（Compliance）
  - `docs/guides/knowledge_space/scenary/SCN-020_contract_quote.md`
  - `docs/guides/knowledge_space/scenary/SCN-130_compliance_regulation.md`

- 研究与资料（Research）
  - `docs/guides/knowledge_space/scenary/SCN-030_research_report.md`

- 台账与清单（Ledger）
  - `docs/guides/knowledge_space/scenary/SCN-040_table_ledger.md`

- 关系与依赖（KG & Dependency）
  - `docs/guides/knowledge_space/scenary/SCN-050_sql_config_kg.md`

- 自定义（Expert）
  - `docs/guides/knowledge_space/scenary/SCN-060_custom_expert.md`

- 产品与商品（Catalog）
  - `docs/guides/knowledge_space/scenary/SCN-070_product_specs.md`
  - `docs/guides/knowledge_space/scenary/SCN-071_product_compat.md`
  - `docs/guides/knowledge_space/scenary/SCN-072_product_selection.md`

- 支持与客服（Support）
  - `docs/guides/knowledge_space/scenary/SCN-080_support_faq.md`
  - `docs/guides/knowledge_space/scenary/SCN-081_support_policy.md`
  - `docs/guides/knowledge_space/scenary/SCN-082_support_troubleshooting.md`
  - `docs/guides/knowledge_space/scenary/SCN-170_ticket_conversations.md`

- 工程与运维（Engineering）
  - `docs/guides/knowledge_space/scenary/SCN-090_eng_runbook.md`
  - `docs/guides/knowledge_space/scenary/SCN-091_eng_incident.md`
  - `docs/guides/knowledge_space/scenary/SCN-092_eng_change.md`

- 数据与接口（Data & API）
  - `docs/guides/knowledge_space/scenary/SCN-100_api_reference.md`
  - `docs/guides/knowledge_space/scenary/SCN-101_data_dictionary.md`

- 销售与市场（Go-to-Market）
  - `docs/guides/knowledge_space/scenary/SCN-110_sales_enablement.md`
  - `docs/guides/knowledge_space/scenary/SCN-120_marketing_promo_rules.md`

- 计费与价格（Billing）
  - `docs/guides/knowledge_space/scenary/SCN-140_billing_pricing.md`

- 协作与项目（Collaboration）
  - `docs/guides/knowledge_space/scenary/SCN-150_meeting_minutes.md`
  - `docs/guides/knowledge_space/scenary/SCN-160_project_docs.md`

- 培训与入职（Enablement）
  - `docs/guides/knowledge_space/scenary/SCN-180_onboarding_training.md`

## 策略包（第二层）

- `docs/guides/knowledge_space/scenary/STR-P0_basic.md`
- `docs/guides/knowledge_space/scenary/STR-P1_general_recommended.md`
- `docs/guides/knowledge_space/scenary/STR-P2_high_accuracy_compliance.md`
- `docs/guides/knowledge_space/scenary/STR-P3_kg_constrained.md`
