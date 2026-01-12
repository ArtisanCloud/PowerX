# PowerX「场景 → 策略包」模式方案（Draft）

> 目的：把 `rag.md` 的“多格式×多策略”落成一个可操作的产品模型：用户先选“场景”，再选该场景下的“策略包”；并支持“自定义场景（专家）”。
>
> 关联：`docs/plan/AI_engineering/knowledge/rag.md`、`docs/plan/AI_engineering/knowledge/knowledage_base.md`、`specs/011-knowledge-space/*`

---

## 1. 结论先行

1. **不做“全量映射”**：不是每个场景都能/都应该使用所有策略。原因是策略依赖不同索引与离线资产（dense/sparse/hier/kg/字段索引），强行全量会导致 UI 难用、实施成本高、用户误选概率高。
2. **映射由“场景的内容结构与任务意图”决定**：场景决定“建库段（Ingestion/Index）要建哪些索引与辅助表”，策略包决定“在线段（RAG）怎么走（护栏/降级/成本）”。
3. **统一入口**：界面只有一个入口（引导模式），第一层场景是“细分业务子场景”（可搜索/分组），并且每个子场景只展示其允许的策略包（非全量映射）。
4. **场景不是“部门/数据源/文件格式”**：一个空间可以混合多个数据源（数据库、文件、网页、API），场景描述的是“主要业务问法/任务意图 + 内容结构假设”，用于决定默认索引与策略组合。

---

## 2. 定义：场景、策略包与三类 Profile

### 2.1 场景（Scene）

场景 = “知识库类型（内容结构）× 主要查询意图（任务类型）”的组合。它主要影响：
- **Ingestion Profile**：解析/切块/增强/脱敏/OCR/embedding 的默认配方
- **Index Profile**：必须启用哪些索引（dense/sparse/hier/kg/字段索引）与所需的辅助表

### 2.2 策略包（Strategy Bundle）

策略包不是单一开关，而是一个可版本化组合：
- Ingestion Profile（可选覆盖）
- Index Profile（可选覆盖）
- RAG Profile（rewrite/recall/fusion/rerank/compress/self-check/CRAG 等）
- Guardrails（成本/质量/合规/降级）

### 2.3 三类 Profile（落地载体）

与 `rag.md` 一致，策略配置拆成三类 Profile：
1) **Ingestion Profile**：按文档格式/结构选择 parser、chunker、augmenter、masking、embedding  
2) **Index Profile**：dense/sparse/hier/kg 是否启用、字段、权重  
3) **RAG Profile**：在线策略组合（rewrite/recall/fusion/rerank/compress/self-check/CRAG）

### 2.4 第三层：RAG 策略模块（L3）

`rag.md` 的 A–O 是“可组合的策略模块”（在线管线节点）。在产品模型里：
- **L1（场景）**决定“建库需要哪些索引/离线资产”
- **L2（策略包）**决定“在线护栏强度/成本等级（P0–P3）”
- **L3（模块）**决定“在线实际启用哪些节点组合（例如 H_fusion / O_crag / K_kg …）”

SSOT：
- 后端映射：`backend/config/knowledge/scene_strategy_catalog.yaml`（`default_modules` / `prerequisites.index`）
- 指导总表：`docs/guides/knowledge_space/scenary/MAP_scene_bundle_rag_profiles.md`

---

## 3. 场景清单（第一层：细分子场景）

> 说明：这里采用“少量大类 + 更多细分子场景”的模型。UI 选择时暴露子场景（更贴近真实问法）；底层映射仍可聚合统计到大类。

### 3.1 大类（仅用于归档/统计）

- 产品与商品（Catalog）
- 支持与客服（Support）
- 工程与运维（Engineering）
- 合同与合规（Compliance）
- 研究与资料（Research）
- 数据与接口（Data & API）
- 关系与依赖（KG & Dependency）
- 自定义（Expert）

### 3.2 细分子场景（UI 选择项）

**产品与商品（Catalog）**
1) 产品库 / 规格参数查询（`product_specs`）  
2) 产品库 / 兼容性与配件关系（`product_compat`）  
3) 产品库 / 选型对比与推荐理由（`product_selection`）

**支持与客服（Support）**
4) 客服 / FAQ 与使用说明（`support_faq`）  
5) 客服 / 售后政策与规则（`support_policy`）  
6) 客服 / 故障现象与排查（`support_troubleshooting`）

**工程与运维（Engineering）**
7) 工程 / Runbook 与标准操作（`eng_runbook`）  
8) 工程 / 故障排查与应急响应（`eng_incident`）  
9) 工程 / 变更与发布（`eng_change`）

**合同与合规（Compliance）**
10) 合同与报价 / 条款与事实（`contract_quote`）

**台账与清单（Ledger）**
11) 台账/清单（表格）/ 记录查询（`ledger_table`）

**研究与资料（Research）**
12) 研究/长报告 / 总结归纳（`research_longdoc`）

**数据与接口（Data & API）**
13) API/接口文档 / 参数与返回（`api_reference`）  
14) 数据字典 / 表结构与字段口径（`data_dictionary`）

**销售与市场（Sales & Marketing）**
15) 销售材料 / 话术与竞品（`sales_enablement`）  
16) 市场活动 / 促销规则（`marketing_promo_rules`）

**法规与计费（Regulation & Billing）**
17) 法规/监管政策 / 口径与约束（`compliance_regulation`）  
18) 计费/价格规则 / 口径与例外（`billing_pricing`）

**协作与记录（Collaboration）**
19) 会议纪要 / 决议与行动项（`meeting_minutes`）  
20) 项目方案 / 交付文档（`project_docs`）

**对话与工单（Conversation & Tickets）**
21) 工单/聊天记录 / 问题追踪（`ticket_conversations`）

**培训与上手（Training）**
22) 入职/培训资料 / 学习路径（`onboarding_training`）

**关系与依赖（KG & Dependency）**
23) SQL/配置/依赖关系（KG 强）（`sql_kg`）

**自定义**
24) 自定义场景（专家）（`custom_expert`）

---

## 4. 策略包清单（第二层）

为了避免“策略太多”，建议先固定 4 类策略包（不同场景可用的子集）：

### P0：基础（不干预 / 最小闭环）
- 目标：跑通 “入库 → 基本召回 → 引用返回”
- 依赖：dense（必须）
- 特点：不开 multiquery/HyDE；不开 rerank；不开 KG；不开 self-check

### P1：通用推荐（企业默认）
- 目标：企业默认可用的综合策略
- 依赖：dense + sparse（强建议）+（可选 hier）
- 组合：hybrid recall + RRF fusion + 轻量 rerank + contextual（按需）
- 说明：这是大多数场景的默认“推荐策略包”

### P2：高准确 / 高风险（合规优先）
- 目标：减少幻觉、强证据、强引用、冲突纠错
- 依赖：sparse（强）+（可选 time-aware）+（可选 CRAG/self-check）
- 组合：sparse 优先 + hybrid + CRAG 或 self-check + must_cite_sources

### P3：KG 约束（关系驱动）
- 目标：以实体/关系为核心，KG 参与 recall/filter/explain
- 依赖：KG（强）+ sparse + dense(摘要)
- 组合：KG recall/filter + hybrid + cite

> 注：P0/P1/P2/P3 是“策略包类型”。各场景会提供其中 2–3 个，而不是全部。

---

## 5. 子场景 → 策略包映射（非全量）

> ✅ 表示默认提供；△ 表示可选扩展（后续版本再开放）；— 表示不建议提供。

| 子场景 | P0 基础 | P1 通用推荐 | P2 高准确/合规 | P3 KG 约束 |
| --- | --- | --- | --- | --- |
| 产品库 / 规格参数查询（product_specs） | — | △ | ✅（默认） | △ |
| 产品库 / 兼容性与配件关系（product_compat） | — | △ | ✅（默认） | ✅（可选） |
| 产品库 / 选型对比与推荐理由（product_selection） | ✅ | ✅（默认） | △ | △ |
| 客服 / FAQ 与使用说明（support_faq） | ✅ | ✅（默认） | △ | — |
| 客服 / 售后政策与规则（support_policy） | — | △ | ✅（默认） | — |
| 客服 / 故障现象与排查（support_troubleshooting） | ✅ | ✅（默认） | △ | △ |
| 工程 / Runbook 与标准操作（eng_runbook） | ✅ | ✅（默认） | △ | △ |
| 工程 / 故障排查与应急响应（eng_incident） | ✅ | ✅（默认） | △ | △ |
| 工程 / 变更与发布（eng_change） | — | △ | ✅（默认） | △ |
| 合同与报价 / 条款与事实（contract_quote） | — | △ | ✅（默认） | △ |
| 台账/清单（表格）/ 记录查询（ledger_table） | ✅ | △ | ✅（默认） | — |
| 研究/长报告 / 总结归纳（research_longdoc） | ✅ | ✅（默认） | △ | — |
| API/接口文档 / 参数与返回（api_reference） | — | △ | ✅（默认） | △ |
| 数据字典 / 表结构与字段口径（data_dictionary） | — | △ | ✅（默认） | △ |
| 销售材料 / 话术与竞品（sales_enablement） | ✅ | ✅（默认） | △ | — |
| 市场活动 / 促销规则（marketing_promo_rules） | — | △ | ✅（默认） | — |
| 法规/监管政策 / 口径与约束（compliance_regulation） | — | △ | ✅（默认） | △ |
| 计费/价格规则 / 口径与例外（billing_pricing） | — | △ | ✅（默认） | — |
| 会议纪要 / 决议与行动项（meeting_minutes） | ✅ | ✅（默认） | △ | — |
| 项目方案 / 交付文档（project_docs） | ✅ | ✅（默认） | △ | — |
| 工单/聊天记录 / 问题追踪（ticket_conversations） | ✅ | ✅（默认） | △ | — |
| 入职/培训资料 / 学习路径（onboarding_training） | ✅ | ✅（默认） | △ | — |
| SQL/配置/依赖关系（KG 强）（sql_kg） | — | △ | △ | ✅（默认） |
| 自定义场景（custom_expert） | ✅ | ✅ | ✅ | ✅ |

### 5.1 为什么不是全量映射（解释你关心的点）

- **合同/报价**不建议提供 P0：因为没有 sparse/结构化/证据护栏时，最容易“答得像真的但错”。  
- **KG 强场景**不建议提供 P0：因为核心价值来自“关系/依赖约束”，没有 KG 就退化为“文本猜测”。  
- **SOP 场景**通常不需要 KG：成本高、收益低；KG 更适合依赖/约束/关系问题。

---

## 5.2 `rag.md` 策略模块 → 场景映射矩阵（第二层“策略”来源）

> 这一节把 `docs/plan/AI_engineering/knowledge/rag.md` 中的策略模块（A1/A2…O）映射到“场景（第一层）→ 可用策略包（第二层）”。
>
> 约定：
> - **默认**：该场景推荐直接启用，UI 默认展示在“推荐策略包”里。
> - **可选**：该场景可提供高级选项/专家开关，但不默认。
> - **不建议**：该场景不建议开放（依赖不匹配/成本收益差/误选风险高）。

### 5.2.1 策略模块依赖速查（会反推“建库需要的索引/辅助表”）

| `rag.md` 模块 | 模块含义（简写） | 主要依赖（Index/Asset） | 典型归属策略包 |
| --- | --- | --- | --- |
| A（Simple RAG） | 简单切块 + 向量召回 | dense | P0 |
| A1 | Query Routing（路由） | 标签/领域路由表（可无索引） | P1/P3（可选） |
| A2 | Time-aware（时间感知） | 时间字段/版本线/冲突策略 | P2（默认于合同） |
| B | Semantic Chunking（语义切块） | 语义边界检测资产 + hier（推荐） | P1（研究类默认） |
| C | Context Enriched（上下文扩展） | hier + 邻接关系（推荐） | P1 |
| D | Doc Augmentation（离线增强） | 摘要/关键词/Q&A/实体标签等离线产物 | P1/P2（合同常用） |
| E | Query Transformation（查询转换） | 规则/LLM（可选） | P1/P2（可选） |
| F | Reranker（重排） | rerank 模型/预算 | P1/P2 |
| G | RSE（语义扩展重排） | 领域词表/实体库（可由 KG 提供） | P2/P3（可选） |
| H | Fusion（融合：vector+bm25+kg+hier） | dense + sparse（强）+（可选 hier/kg） | P1/P2/P3 |
| I | HyDE | LLM+embedding 成本护栏 | P1（研究可选） |
| J | Hierarchical Indices（层次索引） | doc/section summary + 映射关系 | P1（SOP/研究默认） |
| K | Knowledge Graph（KG） | entities/relations/provenance（强） | P3（默认于 KG 场景） |
| L | Feedback Loop | feedback case + 可追溯 chunk_id + reprocess/delta | 全部（企业必备） |
| M | Adaptive RAG | 在线策略路由器 + 可观测 plan | P1/P2/P3（后期） |
| N | Self RAG（自反思回路） | 证据校验 + 最大回路护栏 | P2（高风险可选） |
| O | CRAG（纠错） | 一致性检查器 + 策略切换（sparse/KG） | P2（合同默认） |

### 5.2.2 场景（第一层）→ 推荐策略模块集合（第二层“策略”的偏向型映射）

> 说明：下面列的是“模块集合”。真正落到产品里会以“策略包”呈现（P0/P1/P2/P3），并且只开放该场景允许的子集。

#### 场景 1：SOP/制度/产品说明（Markdown/Word 为主）

- 默认：A（Simple）、H（Fusion=hybrid+RRF）、F（Rerank）、C（Context Enriched）、J（Hier）  
- 可选：E（Query Transformation）、A1（Routing，跨多 space 时）、L（Feedback）  
- 不建议：K（KG 强依赖，通常收益低）

对应策略包：
- 默认 P1；可选 P0（低成本）/P2（高风险 SOP 才开启）

#### 场景 2：产品资料/规格库（Catalog）

- 默认：H（Fusion：dense+sparse）、O（CRAG：事实纠错）、F（Rerank）、E（Query Transformation：字段/单位抽取）、L（Feedback）  
- 可选：A2（Time-aware：版本/有效期）、D（Doc Augmentation：字段���强/同义词/规格摘要）、K（KG-lite：兼容性/配件关系）  
- 不建议：A（纯 Simple，参数类问答风险大）

对应策略包：
- 默认 P2；可选 P1（成本优先）/P3（有明确的关系约束链路时）

#### 场景 3：客服 FAQ / 运营知识

- 默认：H（Fusion）、E（Query Transformation：同义/纠错）、F（Rerank）、C（Context Enriched：前后文补齐）、L（Feedback）  
- 可选：I（HyDE：提升 recall，但要成本护栏）、O（CRAG：高风险纠错）  
- 不建议：K（除非明确需要实体关系）

对应策略包：
- 默认 P1；可选 P0（低成本）/P2（高风险答复）

#### 场景 4：工程/运维/故障排查

- 默认：J（Hier：章节/步骤）、H（Fusion）、F（Rerank）、C（Context Enriched：邻居块/步骤链路）、L（Feedback）  
- 可选：A2（Time-aware：变更窗口/版本）、O（CRAG：高风险纠错）、K（KG：依赖拓扑/影响面）  
- 不建议：A（纯 Simple，排障容易缺上下文）

对应策略包：
- 默认 P1；可选 P0（低成本）/P2（高风险变更）/P3（依赖关系驱动）

#### 场景 5：合同/报价（PDF/Word/Excel 表格多）

- 默认：H（Fusion，sparse 权重大）、A2（Time-aware）、F（Rerank）、O（CRAG）、D（Doc Augmentation）、L（Feedback）  
- 可选：K（KG-lite：实体对齐/条款约束）、E（Query Transformation）、N（Self RAG：极高风险问答）  
- 不建议：A（纯 Simple，风险大）

对应策略包：
- 默认 P2；可选 P1（兜底）/P3（有 KG-lite 时）

#### 场景 6：论文/研究/长报告（PDF 长文）

- 默认：B（Semantic Chunking）、J（Hier-first）、H（Fusion）、F（Rerank）、C（Context Enriched）  
- 可选：I（HyDE）、E（Query Transformation）、L（Feedback）  
- 不建议：K（KG 通常不是第一优先）

对应策略包：
- 默认 P1；可选 P0（低成本）/P2（高风险结论）

#### 场景 7：台账/清单（Excel/CSV 结构化行）

- 默认：H（Fusion：sparse 强 + 行向量）、F（轻量 rerank）、O（CRAG：精确事实纠错）、A2（可选时间过滤）、L（Feedback）  
- 可选：D（字段/单位/币种增强）、E（结构化过滤提取）  
- 不建议：B（语义切块不适用）、J（hier 通常不适用）

对应策略包：
- 默认 P2（精确+证据）；可选 P0（只查粗略）/P1（需要解释归纳时）

#### 场景 8：API/数据字典/接口文档

- 默认：H（Fusion：dense+sparse）、O（CRAG：事实纠错）、E（Query Transformation：参数/字段抽取）、F（Rerank）、L（Feedback）  
- 可选：A2（Time-aware：版本/弃用）、D（Doc Augmentation：示例/字段别名）、K（KG-lite：依赖/调用链）  
- 不建议：A（纯 Simple，事实问答风险大）

对应策略包：
- 默认 P2；可选 P1（解释归纳为主）/P3（依赖关系驱动时）

#### 场景 9：SQL/配置/依赖关系库（KG 强）

- 默认：K（KG 强）、H（Fusion：kg+sparse+dense(摘要)）、F（Rerank 视成本）、C（Context Enriched：结合依赖上下文）、L（Feedback）  
- 可选：A1（Routing：多系统/多域）、A2（Time-aware：版本/变更）、G（RSE：术语/缩写扩展）、O（CRAG：纠错）  
- 不建议：A（纯 Simple，不体现关系价值）

对应策略包：
- 默认 P3；可选 P1（兜底）/P2（高风险变更与合规）

#### 场景 10：自定义场景（专家）

- 默认：无（由用户选择）
- 可选：全部（A1/A2…O），但必须做“依赖校验”（例如选 K 必须已建 KG 索引/表，否则不允许发布为 active）

---

## 5.3 从“模块集合”到“策略包”的落地规则（UI 必须体现）

1. **策略包只暴露偏向型集合**：例如“合同/报价”默认只展示 P2（高准确/合规），而不是让用户在 P0/P1/P2/P3 间乱选。  
2. **策略包与 Index Profile 绑定校验**：P3 需要 KG；P2 需要 sparse/time-aware；P1 强烈建议 sparse/hier；不满足则提示“需要先建索引/运行体检/安装插件”。  
3. **自定义场景才开放全量模块**：并且强制依赖校验 + 成本护栏（HyDE/rerank/self loop）。

---

## 6. 场景会“反推建库分段（segment）”需要哪些索引/辅助表

> 你提到的“除了向量库，还要建辅助表做索引（比如 KG）”，就在这一层落地。

### 6.1 每个场景的 Index Profile 最小要求

1) SOP/制度：dense + sparse + hier（推荐）  
2) 合同/报价：dense + sparse（强）+（表格字段索引）+（time-aware）  
3) 论文/研究：dense + hier（强）+（semantic chunking 资产）  
4) 台账/清单：sparse（强）+ 行级 dense + 字段索引（强）  
5) SQL/配置（KG 强）：KG（强）+ sparse + dense(摘要) + 依赖/实体索引表  

### 6.2 KG 场景需要的最小辅助表/资产（MVP）

- `entities`（实体：表/字段/服务/版本/术语…）
- `relations` / `triples`（关系：依赖/属于/约束/替换…）
- `entity_alias`（同义/别名）
- `provenance`（关系/实体溯源到 chunk_id/source_uri）
- （可选）`graph_index`（加速查找）

---

## 7. “自定义场景（专家）”怎么定义才不与 P0 混淆

### 7.1 自定义场景是什么
- 自定义场景 = 你可以自己定义：内容结构假设、主要意图、默认索引通道、chunking 配方、以及可用策略包集合。

### 7.2 P0（基础策略包）是什么
- P0 是一个“策略包类型”，它强调“最少干预、最小闭环”，并不等于“专家自定义”。

> 结论：把“自定义”放在场景层（Scene=自定义），把“基础/通用/高准确/KG”放在策略包层（Bundle），两者不冲突。

---

## 8. Chunking（分段/分隔符/overlap）的可配置项（你提到的 800 与 delta）

这些配置属于 **Ingestion Profile** 的“chunker”模块。建议 UI 表达为：

- `chunk_size`：默认 800（可选单位：字符；工程实现建议用 tokens）
- `chunk_overlap`：默认 120（你说的 delta：前后重叠）
- `separators`：分隔符优先级列表（按场景给默认值）
- `min_chunk_size` / `max_chunk_size`
- `context_neighbors`：是否在检索阶段自动补前后邻居块（属于 contextual）

按场景给默认值（示意）：
- SOP：`chunk_size=800`、`overlap=120`、`separators=["\\n\\n","\\n","。","；"]`
- 论文：`chunk_size=1200`、`overlap=200`、`separators=["\\n\\n","\\n","。"]` + semantic chunking 开关
- 台账：行级切块（不以字符切），`overlap=0`
- SQL：按对象/函数/语句块切（不以字符切），`overlap=0` + 依赖抽取开关

---

## 9. 下一步（用于逐一开发）

建议把实现按“先骨架后策略”推进：
1) 先实现场景与策略包的数据结构与 UI 流程（只显示映射允许的选项）  
2) 再逐场景补齐 Index Profile 的“索引/辅助表”能力（尤其 sparse/hier/kg）  
3) 最后把 Corpus Check 推荐卡片挂到“场景/策略包”的选择上（只推荐该场景允许的策略包）
