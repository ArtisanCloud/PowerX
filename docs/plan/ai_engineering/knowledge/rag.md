# PowerX RAG 设计方案（多格式企业文档 + 多策略检索增强生成）

> 版本：v0.1（草稿）  
> 目标：在 PowerX Knowledge Space 底座上，支持企业多类型文档入库与多种 RAG 策略可配置组合；上层智能体框架可替换（Eino 只是默认适配之一）。  
> 关联文档：`docs/plan/ai_engineering/knowledge/knowledage_base.md`、`docs/plan/ai_engineering/knowledge/chunking_strategy.md`、`specs/011-knowledge-space/*`

---

## 1. 核心结论（先说清楚）

1. **“文档类型 × 任务类型”决定 RAG 方案**：不同格式/结构的内容需要不同的解析、分块、向量化与检索策略；RAG 策略必须是可配置组合，而不是单一固定流程。
2. **底座必须多索引并存**：仅靠“chunk 向量”无法覆盖所有场景；至少要同时具备：  
   - 向量索引（dense）  
   - 关键词/稀疏索引（sparse/BM25/FTS）  
   - 层次化索引（doc→section→chunk→sentence 的多粒度）  
   - 知识图谱索引（实体/关系/事件，必须具备）
3. **策略以模块化管线表达**：把 RAG 拆成 `rewrite → recall → fusion → rerank → compress → answer → feedback` 七段，每段提供可选算法；策略版本化，支持灰度与回滚。

---

## 2. 输入资产：企业文档的分类与“可检索化”要求

### 2.1 按格式分类（Format）

| 格式 | 典型示例 | 主要难点 | 解析/抽取建议 |
| --- | --- | --- | --- |
| Word（doc/docx） | SOP、方案、报价单 | 标题层级、表格、页眉页脚、水印 | 保留标题层级；表格抽取为结构化块；去噪（页眉页脚） |
| PDF | 合同、论文、扫描件 | 版面、两栏、图表、OCR | 优先文本层；无文本层走 OCR；保留页码/段落位置 |
| PPT | 培训/宣讲 | “要点式”短文本、图多 | 每页为一个 section；讲稿/备注优先；图走 OCR/Caption |
| Excel（xls/xlsx） | 价格表、台账、清单 | 行列语义、合并单元格、公式 | 以“表→行→单元格”层次建索引；行级 embedding + 关键列字段化 |
| CSV | 导出报表 | 无 schema/列含义弱 | 需先推断 schema；按行/分组 embedding；保留列名与类型 |
| TXT/Markdown | 笔记、说明文档 | 结构较好 | Markdown 按标题切；代码块单独处理 |
| HTML/网页 | Wiki/知识库页面、产品站点 | 噪声（导航/脚本）、富文本、链接结构 | DOM 抽取正文；保留标题层级与链接；把 URL 与更新时间写入 metadata |
| 邮件（eml/msg） | 合同往来、确认函、报价沟通 | 线程结构、引用回复、附件 | 按 thread 切块（subject/from/to/date）；引用内容降权；附件走独立 source |
| IM/聊天记录（txt/json） | 工单沟通、群聊纪要 | 多人对话、噪声、敏感信息 | 按时间窗/话题聚合；参与人/频道写入 metadata；默认启用 masking/ACL |
| JSON/YAML/配置 | 产品配置、部署参数 | 层级结构、环境差异 | 结构化展开（path=value）；按对象/段落切块；保留 env/tag |
| 图片（png/jpg） | 扫描件、截图 | OCR 质量、图表 | OCR + 图表结构化（可后续）；保留坐标与置信度 |
| SQL 文件 | DDL/DML、存储过程 | 可编译结构、依赖关系 | AST 解析；按对象（表/视图/函数）分块；抽取依赖图 |
| 日志/半结构 | log/json | 噪声、重复 | 先采样/聚类；按事件模板聚合；避免全量入库 |
| 代码仓库（git） | README、源码、设计文档 | 多文件关联、目录语义 | 以“文件→符号→块”层次索引；目录/包路径写入 metadata；可选生成 repo 摘要 |
| 音频/视频（可选） | 会议录音、培训视频 | 需要 ASR、时间轴 | ASR 转写为段落块（带时间戳）；可按“发言人/议题”再切块；时间戳作为 provenance |

> 设计原则：解析阶段必须把内容拆成“可追溯单元”（doc_id/section_id/chunk_id），并保留 **来源定位信息**（page、slide、sheet、row、line_range、bbox）。

### 2.2 按结构分类（Structure）

| 类型 | 示例 | 建议索引单元 | 关键 metadata |
| --- | --- | --- | --- |
| 非结构化长文 | 论文、案例、产品说明 | section/chunk/sentence 多粒度 | 标题路径、页码、段落序号、引用/脚注 |
| 半结构化 | SOP、规章、FAQ | “条款/步骤/问答对” | 章节编号、步骤号、角色、前置条件 |
| 结构化表格 | 价格表、库存、台账 | 行/记录为主，必要时单元格 | 主键列、关键列、单位、币种、时间、数据域 |
| 代码/配置 | SQL、YAML、JSON、代码片段 | 对象/函数/块 | symbol、依赖、路径、版本、环境（dev/prod） |

### 2.3 “任务类型”标签（Query Intent）

企业 RAG 的检索任务通常可归为：
- **事实查找**：找具体数值/条款/步骤（偏 sparse/精确定位）
- **解释归纳**：基于多段材料总结（偏 dense + 压缩/摘要）
- **对比决策**：方案对比、报价对比（需要结构化抽取 + 表格检索 + rerank）
- **流程执行**：SOP/操作指南（需要层次结构与步骤保真）
- **根因排查**：日志/告警解释（需要聚类/模板化）
- **依赖追踪**：SQL/schema/系统关系（需要 KG/依赖图）

> UI/策略选择应允许为每个 Knowledge Space 声明 “主要任务类型”，作为默认策略模板选择依据。

---

## 3. 索引体系（Indexing）：必须是“多索引协同”

### 3.1 四层索引（最低要求）

1) **Dense 向量索引（chunk vector）**  
用于语义召回，覆盖“解释归纳/相似问法”。

2) **Sparse / 关键词索引（BM25/FTS）**  
用于精确名词、编号、字段值查找；对表格/条款编号特别重要。

3) **Hierarchical Indices（层次化索引）**  
至少三层：`doc_summary → section_summary → chunk`  
用于“先定位章节再下钻”，降低噪声、提升可解释性。

4) **Knowledge Graph（知识图谱，必须要有）**  
从文档中抽取实体/关系/事件：  
`实体（产品/人/组织/合同/表/字段/系统）` + `关系（依赖/属于/约束/版本/替换）`  
用于路由检索、约束过滤、跨文档对齐与解释链输出。

### 3.2 建议的入库产物（Artifacts）

每次 Ingestion Job 至少产出：
- `chunk_manifest`：chunk 文本 + metadata（定位信息、标签、ACL、hash）
- `vector_manifest`：chunk_id → vector（模型、维度、版本）
- `hier_manifest`：doc/section 的摘要文本与映射关系
- `kg_manifest`：entities/relations/triples + provenance（来源 chunk_id）
- `masking_report`（可选）：PII/敏感字段覆盖率

---

## 4. 分块与向量化策略（按文档维度选择）

### 4.1 分块（Chunking）策略矩阵

| 场景 | 推荐 chunking | 说明 |
| --- | --- | --- |
| Markdown/SOP/说明文档 | **结构化切块**（按标题/列表/条款） | 保真度高，便于定位与引用 |
| 长论文/研究报告 | **Semantic Chunking**（语义边界）+ 章节锚点 | 避免跨主题；保留 section path |
| 合同/报价单 | **条款/字段切块** + 表格行切块 | 要支持精确召回（编号、金额、单位） |
| PPT | **按页** + 每页要点子块 | 页为 section，子块为 bullet |
| Excel/CSV | **行级** + “关键列拼接” | 行是检索单元；列名进入文本 |
| SQL/代码 | **按对象/函数/块** | AST 优先；保留 symbol/依赖 |
| 图片/PDF 扫描 | OCR 段落切块 + bbox | 必须保留页码/坐标与置信度 |

### 4.2 向量化（Embedding）策略要点

1. **空间内 embedding 建议固定**：同一 space 不建议混不同维度/不同模型；确需混用时必须按 `embedding_profile_id` 分桶索引。
2. **表格要“结构化再向量化”**：将 `列名:值` 展开为文本，或生成“行描述”；同时保留结构化字段用于过滤。
3. **代码/SQL 用“对象摘要 + 关键 token”**：仅向量化注释/对象描述/关键字段；原始代码以 sparse/语法索引补充。
4. **图片/OCR 需置信度门控**：低置信度 chunk 不进入主索引或降权；避免污染召回。

---

## 5. RAG 策略模块化（把你列的方式落到可组合管线）

### 5.1 统一在线管线（Online Pipeline）

```mermaid
flowchart LR
  Q["User Query + Context"] --> RW["Query Transformation\n(rewrite/hyde/router)"]
  RW --> RC["Recall\n(vector/bm25/kg/hier)"]
  RC --> FU["Fusion\n(rrf/weights/dedup)"]
  FU --> RR["Rerank\n(cross-encoder/llm/rse)"]
  RR --> CP["Compress/Context Build\n(extract/summary/contextual)"]
  CP --> AN["Answer\n(generate + cite + tool)"]
  AN --> FB["Feedback Loop\n(label/reprocess/rollback)"]
```

PowerX 的实现建议让 `KnowledgeQueryService` 直接产出 `RetrievalPlan`（每一步耗时/参数/候选数）供 UI/审计使用。

### 5.2 策略包清单（A0–O）：定义“是什么/何时用/依赖什么”

下面把你列的策略映射到管线阶段，并给出落地依赖。**A0–O 本身就是“策略包候选”（可单独作为 UI 选择项）**；P0–P3 仅作为“组合档位/默认预设”的封装方式，不与 A0–O 同层级。

#### A0. Metadata Filtering / ACL-first Retrieval（元数据过滤与权限优先）
- **阶段**：Recall（前置过滤）/ Fusion（过滤后归一化）
- **做法**：先用 `tenant_uuid + space_id + acl_tags + time_range + doc_type + department_code` 等做过滤，再做向量/BM25/KG 召回
- **适用**：企业场景通用默认（合规与降噪的基座能力）
- **依赖**：chunk metadata 规范化；查询层强制 enforce ACL（不可只靠 UI/上层）

#### A1. Query Routing（查询路由）
- **阶段**：Rewrite / Recall
- **做法**：先做意图分类/领域判断，把 query 路由到不同的索引通道或不同 space（合同/报价/产品/SOP/SQL/日志）
- **适用**：多知识域、多类型库混用场景；比“全库检索”更稳、更省成本
- **依赖**：意图分类器（规则+LLM）；路由表（space/tag/领域）与可观测 plan

#### A2. Time-aware RAG（时间感知检索）
- **阶段**：Recall / Fusion
- **做法**：对政策/报价/版本文档按更新时间做衰减或“仅最新版本”；支持 `effective_from/to` 与版本线
- **适用**：价格、政策、产品版本频繁变更的库
- **依赖**：文档版本化与时间字段；Fusion 时做时间权重/冲突策略

#### A. Simple RAG（简单切块）
- **阶段**：Chunking + Vector Recall
- **适用**：内容较干净、结构明确（Markdown/FAQ/SOP）
- **依赖**：chunk 向量索引
- **风险**：噪声大时幻觉；对精确字段查询弱

#### B. Semantic Chunking（语义切块）
- **阶段**：Chunking
- **适用**：长论文、研究报告、长方案
- **依赖**：语义边界检测（可用 embedding 相邻差分/LLM 辅助）
- **要点**：必须保留章节锚点（section path），否则引用难追溯

#### C. Context Enriched Retrieval（上下文增强检索）
- **阶段**：Recall / Compress
- **做法**：召回 chunk 后，向上扩展同 section 的邻居块/父摘要；或拼接“标题路径 + 摘要 + chunk”
- **适用**：SOP/规范/条款需要“前后文”
- **依赖**：层次化索引与 chunk 邻接关系

#### D. Document Augmentation（文档增强）
- **阶段**：Indexing（离线增强为主）
- **做法**：对 chunk 生成增强字段：关键词、摘要、问题对、实体标签、同义词、结构化字段
- **适用**：合同/报价单/产品说明（字段密集）
- **依赖**：离线 pipeline + artifact 版本管理

#### E. Query Transformation（查询转换）
- **阶段**：Rewrite
- **子类**：MultiQuery、意图分类、同义词扩展、结构化过滤提取（时间/金额/型号）
- **适用**：用户问法多样、口语化
- **依赖**：LLM/规则；输出应进入审计（可解释）

#### F. Reranker（重排序）
- **阶段**：Rerank
- **适用**：召回候选多且相似，尤其是 Hybrid 场景
- **依赖**：cross-encoder 或 rerank LLM；成本可控（topN）

#### G. RSE（Re-ranking with Semantic Expansion）
- **阶段**：Rerank（带扩展）
- **做法**：对候选/查询做语义扩展（同义、实体补全、标题路径补全）再 rerank
- **适用**：专业术语、缩写、型号多的领域
- **依赖**：领域词表/实体库（可由 KG 提供）

#### H. Fusion（融合检索）
- **阶段**：Fusion
- **做法**：vector + bm25 + kg + hier 多路召回 → RRF/加权/去重
- **适用**：企业知识库通用默认（强烈建议）
- **依赖**：多索引并存；统一 score 归一化

#### I. HyDE（Hypothetical Document Embedding）
- **阶段**：Rewrite
- **做法**：先让 LLM 生成“假设答案/假设文档”，对该文本向量化再检索
- **适用**：问题抽象、缺少关键词时提升 recall
- **依赖**：LLM + embedding；需防止生成偏题（加入约束/自检）

#### J. Hierarchical Indices（层次化索引）
- **阶段**：Recall / Context
- **做法**：先召回 doc_summary/section_summary，再下钻 chunk；或并行召回并融合
- **适用**：长文档与多章节手册、SOP、规范
- **依赖**：离线摘要索引（doc/section）与映射关系

#### K. Knowledge Graph（知识图谱）
- **阶段**：Recall / Filter / Explain
- **做法**：实体识别 → KG 查询（相关实体/依赖/约束）→ 限定召回范围或生成结构化上下文
- **适用**：依赖追踪（SQL/schema/系统关系）、跨文档对齐、合规约束
- **依赖**：KG 构建 pipeline（实体/关系抽取 + provenance）与图查询能力

#### L. Feedback Loop（反馈闭环）
- **阶段**：Feedback
- **做法**：用户/运营标注 → 触发 reprocess（重分块/重向量化/策略回滚）→ 指标回归
- **适用**：企业场景必须（质量治理）
- **依赖**：FeedbackCase + 可追溯 chunk_id + reprocess/delta 工具链

#### M. Adaptive RAG（自适应检索增强生成）
- **阶段**：路由/动态策略
- **做法**：根据 query 复杂度/置信度/成本预算，动态选择：是否 HyDE、是否 rerank、topK、是否 KG
- **适用**：流量大、成本敏感、任务多样
- **依赖**：在线策略路由器（Rule+LLM）、可观测与门控指标

#### N. Self RAG（自反思检索增强生成）
- **阶段**：Answer（带反思回路）
- **做法**：先生成草案→自检（缺证据/冲突/不确定）→ 触发二次检索/改写→再回答
- **适用**：高风险问答（合规/合同/财务）、需要降低幻觉
- **依赖**：反思提示词、证据校验器、最大回路次数限制

#### O. CRAG（Corrective RAG）
- **阶段**：Answer / Feedback（纠错）
- **做法**：检测回答与证据不一致/证据不足→调整检索策略或请求更精确证据（如走 sparse/KG）
- **适用**：精确事实类问题、合规类问题
- **依赖**：一致性检查器（规则/LLM）、可解释的候选证据集

---

## 6. “多格式×多策略”的落地方式：Profile 化

建议把策略配置拆成三类 Profile，便于 UI 与默认值管理：

1) **Ingestion Profile（入库配置）**：按文档格式/结构选择 parser、chunker、augmenter、masking、embedding
2) **Index Profile（索引配置）**：dense/sparse/hier/kg 是否启用、维度、字段、权重
3) **RAG Profile（在线策略）**：rewrite/recall/fusion/rerank/compress/self-check 的组合

### 6.1 推荐的默认模板（示例）

| 场景 | Ingestion Profile | Index Profile | RAG Profile |
| --- | --- | --- | --- |
| SOP/产品说明（Markdown/Word） | 结构化切块 + 摘要增强 | dense + sparse + hier | hybrid + rrf + rerank + contextual |
| 合同/报价单（PDF/Word/Excel） | 条款/表格行切块 + 字段抽取 | dense + sparse（强） + hier | sparse 优先 + hybrid + CRAG + cite |
| 论文/研究（PDF） | semantic chunking（智能语义=LLM，当前未接入）+ section 摘要 | dense + hier | hier-first + HyDE(可选) + rerank |
| 台账/清单（Excel/CSV） | 行级切块 + schema 推断 | sparse + dense（行向量） | 精确过滤 + rerank(轻量) |
| SQL/配置库 | AST 切块 + 依赖抽取 | sparse + KG（强） + dense(摘要) | KG 约束检索 + hybrid + cite |

---

## 7. Web Admin UI：如何让“选策略”可用且可控

### 7.1 关键页面

1. **Space → Profiles**：为该空间选择/编辑 Ingestion/Index/RAG 三类 profile（版本化、可回滚）
2. **Ingestion Job 详情**：展示 chunking/embedding/augment 结果统计（覆盖率、错误、低置信度比例）
3. **Retrieval Playground**（核心）：  
   - 选择 space + RAG profile（或临时 override）  
   - 展示 RetrievalPlan（每段耗时/候选数/参数）  
   - Candidates（多路召回来源标识：vector/bm25/kg/hier）  
   - Rerank 前后对比（可视化 delta）  
   - 最终 ContextPack（token 预算、压缩结果、引用映射）
4. **Feedback**：从回答页一键生成 FeedbackCase（自动带 trace_id + chunk_id），支持触发 reprocess/rollback

### 7.2 策略护栏（Guardrails）

- 成本护栏：`max_queries_num`、`hyde_enabled`、`rerank_topk`、`self_rag_max_loops`
- 质量护栏：`min_evidence_chunks`、`must_cite_sources`、`consistency_check=on`
- 合规护栏：`enforce_acl=true`、`masking_required=true`（未达标 job blocked）
- 降级策略：当 embedding/向量库不可用时自动切 `sparse-only` 并提示降级原因

### 7.3 引导式 + 默认式推荐（让用户“选得对、改得动”）

为了避免用户面对大量策略无从下手，UI 建议提供“双轨”交互：

**默认式（模板化）**
- 用户在创建 Space 或导入数据时只需要选择“场景模板”（如 SOP/制度、合同/报价、产品手册、SQL/数据字典、会议纪要、台账清单）
- 系统自动选择并锁定一套默认组合：`Ingestion Profile + Index Profile + RAG Profile`
- 默认组合必须“能跑通最小闭环”，并内置 Guardrails（topK、rerank_topk、HyDE 开关、最大回路次数等）

**引导式（向导 + 语料体检 Corpus Check）**
- 在导入首批样本文档后（建议 20~50 份或 50MB 内），执行一次“语料体检”，产出可解释建议：
  - 文档格式占比（pdf/word/xlsx/html/img…）
  - 扫描/OCR 占比（文本层缺失率）
  - 表格占比、代码占比、平均段落长度、重复率
  - 语言分布（中文/英文/双语）
  - 敏感信息风险（是否建议强制 masking/ACL）
- UI 以“推荐策略卡片”呈现：推荐的 Profile 组合 + 预计成本影响（是否启用 HyDE、是否启用 rerank 等）+ 风险提示

**最小可实现的推荐规则（示例）**
- SOP/制度/产品手册：`hybrid + rrf + rerank + contextual`，chunking=结构化，hier=on
- 合同/报价：`sparse 优先 + hybrid + CRAG + must_cite_sources`，表格行索引=on，time-aware=on
- 论文/研究：`hier-first + semantic chunking（智能语义=LLM，当前未接入）+ rerank + HyDE(可选)`，cite=on
- SQL/数据字典：`KG 约束检索 + hybrid + cite`，AST 切块=on
- 台账/清单：`精确过滤 + sparse + 轻 rerank`，行级索引=on

---

## 8. 对 PowerX 的实现建议（接口形态）

### 8.1 面向在线的统一输出：RetrievalPlan + ContextPack

为了支持 Adaptive/Self/CRAG，以及 UI 可解释性，建议 Query API 返回：
- `plan`: 每个阶段的输入参数、耗时、候选数、降级原因
- `candidates`: 分来源的候选（含 score、chunk_id、provenance）
- `context_pack`: 最终上下文（已压缩/摘要/去重），附 citations 映射
- `trace_id`: 用于审计/反馈闭环

### 8.2 底层表/资产设计的通用性（兼容多策略的关键）

大部分 RAG 策略不需要新增业务表，而是“对同一批通用资产进行不同编排”。底层需要保证以下资产与字段稳定存在（细节可对齐 `specs/011-knowledge-space/data-model.md`）：

**Chunk（通用召回单元）**
- 必要字段：`chunk_id`、`doc_id`、`space_id`、`tenant_uuid`、`content`、`content_hash`
- 必要 metadata（建议 jsonb）：`doc_type/format`、`title_path`、`language`、`updated_at`、`source_uri`、`provenance(page/slide/sheet/row/line_range/bbox/timecode)`、`acl_tags`、`confidence`（OCR/ASR）

**Embedding（允许多 profile，但默认单一）**
- 必要字段：`chunk_id`、`embedding_profile_id`、`dimensions`、`vector`、`model`、`created_at`
- 约束：同一 space 默认只启用一个 embedding_profile；若启用多个，查询必须显式指定 profile

**Sparse/Text Index（可选，但企业场景强烈建议具备）**
- 最小要求：可按 `chunk_id` 建倒排或 FTS，并支持过滤条件一致（tenant/space/acl/time）

**Hier（层次化摘要）**
- 最小要求：`doc_summary`、`section_summary` 与 `parent-child` 映射到 chunk

**KG（知识图谱）**
- 最小要求：`entity`、`relation/triple`、`provenance(chunk_id)`、`confidence`

**策略与运行记录（支撑 Adaptive/Self/CRAG/回放）**
- 保存 `policy_version_snapshot`、`retrieval_plan`、`trace_id`、`degrade_reason`，用于可解释、回放与反馈闭环

> 结论：底层采用通用资产方案即可兼容 Simple/Semantic/HyDE/Fusion/Rerank/Self/CRAG/Adaptive/KG 等策略；新增策略主要是新增“管线节点与配置”，而不是新增存储模型。

### 8.3 知识图谱的最小可用（MVP）

MVP 不追求“全自动高质量 KG”，先验证可用闭环：
- 抽取：实体（产品/组织/系统/表/字段/版本/合同编号）+ 关系（依赖/属于/约束/替换）
- 存储：图数据库或 Postgres 图表（先 MVP 可用）
- 查询：给定 query → 命中实体 → 返回相关实体与关联 chunk_id（provenance）
- 用法：作为 recall 的一条通道 + 作为过滤/解释输出

### 8.4 OCR/ASR 与格式转换：建议“底座接口 + 可选插件实现”

企业文档中扫描 PDF、图片、甚至音视频都很常见，但 OCR/ASR 往往带来额外依赖与成本。推荐方案：

**底座固化什么**
- 固化的是“能力接口与编排能力”，而不是某个 OCR 引擎：
  - `DocumentProcessor`/`OCRProvider`/`ASRProvider` 的接口定义
  - Ingestion pipeline 中对 `needs_ocr`、`confidence`、`blocked/degraded` 的统一处理
  - UI 能展示“为什么需要 OCR、OCR 覆盖率、置信度分布、降级原因”

**插件实现什么（推荐）**
- 由插件（例如 `com.powerx.plugin.data_forge`）提供：
  - 图片/PDF 扫描件 OCR（输出文本 + bbox + 置信度）
  - Office/网页抽取与格式清洗（可选）
  - 结构化抽取（表格/字段）（可选）
- 插件通过 Capability 注册给 Knowledge Space ingestion 调用（与 agent 框架无关）

**避免“必须安装插件很麻烦”的产品策略**
- PowerX 不把 `com.powerx.plugin.data_forge` 设为强制依赖：默认可在“无 OCR”模式运行（只处理有文本层的 PDF/Office/文本）。
- 当检测到 `扫描占比高/图片占比高` 时：
  - UI 给出明确提示：当前数据需要 OCR 才能获得可用召回质量
  - 提供“一键启用 OCR 扩展”（SaaS 场景可预置；私有化场景提示安装/配置）
- 任务行为建议：
  - `ocr_required=true` 或 `masking_required=true` 的 space：OCR 不可用时 Job 进入 `blocked`（合规优先）
  - 非强制：OCR 不可用时 Job 进入 `degraded`，并降低该批次 chunk 的权重或不入主索引

> 建议落地：把 `data_forge` 定位为“Recommended 插件”，并在安装/向导/体检阶段自动引导；SaaS 可由平台预装实现“开箱即用”，私有化则允许接入客户已有 OCR 服务地址，减少安装负担。

---

## 9. 分阶段落地路线（建议）

**Phase 1（可用）**  
Simple RAG + Hybrid（dense+sparse）+ RRF + 基础 rerank；支持 Word/PDF/Markdown/CSV 入库；Playground 可解释。

**Phase 2（好用）**  
Semantic chunking、Hierarchical indices、HyDE、Context-enriched retrieval、反馈闭环（reprocess/rollback）。

**Phase 3（强大）**  
Knowledge Graph 深化 + Adaptive RAG（动态路由）+ Self/CRAG（纠错/自反思）+ 领域词表/实体库（RSE）。
