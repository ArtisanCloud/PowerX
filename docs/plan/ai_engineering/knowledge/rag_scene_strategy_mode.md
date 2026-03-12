# PowerX「策略包 → 场景」模式方案（Draft）

> 目的：把 `rag.md` 的策略包（A0–O）作为第一层选择项；场景只作为“适用范围/映射说明”，不再承担第一层筛选职责。
>
> 关联：`docs/plan/ai_engineering/knowledge/rag.md`、`docs/plan/ai_engineering/knowledge/knowledage_base.md`、`docs/plan/ai_engineering/knowledge/chunking_strategy.md`、`specs/011-knowledge-space/*`

---

## 1. 结论先行

1. **策略包优先**：UI 第一层只展示 A0–O 策略包（来自 `rag.md` 5.2），用户先选策略包，再看其适用场景。
2. **场景作为说明/过滤**：场景不再是第一层必选项；只用于展示“此策略包适用于哪些场景”。
3. **不再设 L2/L3**：策略包（A0–O）就是主线；P0–P3 仅保留为“组合档位/默认预设”，不作为 UI 主线层级。

---

## 2. 定义

### 2.1 策略包（Strategy Package）

策略包 = `rag.md` 5.2 中的 A0–O。  
每个策略包定义：**阶段/做法/依赖/适用场景**。  
UI 可直接把 A0–O 作为可选项。

### 2.2 场景（Scene）

场景 = 内容结构 + 查询意图。  
在此文档中，场景只用于“策略包适配说明”，不作为强制筛选层级。

### 2.3 Profile（落地载体）

1) **Ingestion Profile**：解析/切块/增强/脱敏/OCR/embedding 的默认配方  
2) **Index Profile**：dense/sparse/hier/kg 是否启用、字段、权重  
3) **RAG Profile**：在线策略组合（rewrite/recall/fusion/rerank/compress/self-check/CRAG）

---

## 3. 策略包清单（A0–O）

（仅列出名称，详细解释见 `rag.md` 5.2）

- A0. Metadata Filtering / ACL-first Retrieval  
- A1. Query Routing  
- A2. Time-aware RAG  
- A. Simple RAG  
- B. Semantic Chunking  
- C. Context Enriched Retrieval  
- D. Document Augmentation  
- E. Query Transformation  
- F. Reranker  
- G. RSE  
- H. Fusion  
- I. HyDE  
- J. Hierarchical Indices  
- K. Knowledge Graph  
- L. Feedback Loop  
- M. Adaptive RAG  
- N. Self RAG  
- O. CRAG

---

## 4. 场景清单（用于映射说明）

- product_specs：产品库 / 规格参数查询  
- product_compat：产品库 / 兼容性与配件关系  
- product_selection：产品库 / 选型对比与推荐理由  
- sop：SOP/制度/产品说明  
- contract_quote：合同/报价  
- research_longdoc：论文/研究/长报告  
- ledger_table：台账/清单（表格）  
- support_faq：客服 / FAQ 与使用说明  
- support_policy：客服 / 售后政策与规则  
- support_troubleshooting：客服 / 故障现象与排查  
- eng_runbook：工程 / Runbook 与标准操作  
- eng_incident：工程 / 故障排查与应急响应  
- eng_change：工程 / 变更与发布  
- api_reference：API/接口文档 / 参数与返回  
- data_dictionary：数据字典 / 表结构与字段口径  
- sales_enablement：销售材料 / 话术与竞品  
- marketing_promo_rules：市场活动 / 促销规则  
- compliance_regulation：法规/监管政策 / 口径与约束  
- billing_pricing：计费/价格规则 / 口径与例外  
- meeting_minutes：会议纪要 / 决议与行动项  
- project_docs：项目方案 / 交付文档  
- ticket_conversations：工单/聊天记录 / 问题追踪  
- onboarding_training：入职/培训资料 / 学习路径  
- sql_kg：SQL/配置/依赖关系（KG 强）  
- custom_expert：自定义（专家）

---

## 5. 策略包 → 场景映射（Draft）

> 说明：此映射用于 UI 展示“适用场景”；并不强制限制场景选择（可在后续版本中加白名单开关）。

- A0（Metadata/ACL）  
  适用场景：全部场景（基础能力，默认建议开启）

- A1（Query Routing）  
  适用场景：跨多个知识域/多空间检索（如产品 + 客服 + 工程 + 合规并存）

- A2（Time-aware）  
  适用场景：contract_quote、compliance_regulation、billing_pricing、marketing_promo_rules、product_specs

- A（Simple RAG）  
  适用场景：sop、support_faq、support_policy、support_troubleshooting、eng_runbook、meeting_minutes、project_docs、onboarding_training

- B（Semantic Chunking）  
  适用场景：research_longdoc、project_docs、meeting_minutes

- C（Context Enriched）  
  适用场景：sop、contract_quote、eng_runbook、support_policy、compliance_regulation

- D（Doc Augmentation）  
  适用场景：contract_quote、product_specs、sales_enablement、marketing_promo_rules、compliance_regulation

- E（Query Transformation）  
  适用场景：product_specs、data_dictionary、api_reference、billing_pricing

- F（Reranker）  
  适用场景：product_specs、product_compat、contract_quote、compliance_regulation、data_dictionary、api_reference

- G（RSE）  
  适用场景：product_specs、product_compat、data_dictionary、api_reference、sql_kg

- H（Fusion）  
  适用场景：除“极简场景”外的多数场景（产品/客服/工程/合规/数据/销售/市场/会议/项目/工单/培训/SQL）

- I（HyDE）  
  适用场景：research_longdoc、project_docs、meeting_minutes

- J（Hierarchical Indices）  
  适用场景：sop、research_longdoc、eng_runbook、compliance_regulation

- K（Knowledge Graph）  
  适用场景：product_compat、data_dictionary、api_reference、sql_kg、compliance_regulation

- L（Feedback Loop）  
  适用场景：全部场景（企业必备）

- M（Adaptive RAG）  
  适用场景：全部场景（高流量/多策略时启用）

- N（Self RAG）  
  适用场景：contract_quote、compliance_regulation、billing_pricing

- O（CRAG）  
  适用场景：contract_quote、compliance_regulation、billing_pricing、product_specs

---

## 6. 策略包与分割策略的联动强度（Draft）

> 目的：明确“策略包配置页”和“分割策略页”的耦合程度，避免误用。

### 6.1 弱关联（策略在检索/融合阶段，分割可独立）

- **A** 简单 RAG  
- **A0** 元数据过滤  
- **C** 上下文增强  
- **F** 重排  
- **H** 融合检索  
- **A1** 查询路由  
- **E** 查询转换  
- **G** 语义扩展重排  
- **I** HyDE  
- **L** 反馈闭环  
- **M** 自适应 RAG  
- **N** 自反思回路

### 6.2 强关联（建议同页联动 + 给出推荐分割）

- **B 语义切块**：直接定义切块方式与边界策略  
- **J 层次索引**：要求章节/层级锚点  
- **D 文档增强**：依赖稳定块边界与结构化锚点  
- **A2 时间感知**：时间字段需稳定写入 metadata（分割应保留字段）  
- **K 知识图谱**：需要实体/关系抽取的稳定块与锚点  
- **O CRAG**：依赖“证据块”稳定性（分割推荐需明确）
