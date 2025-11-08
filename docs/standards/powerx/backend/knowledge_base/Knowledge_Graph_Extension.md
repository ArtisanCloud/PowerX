# 知识图谱扩展（Knowledge_Graph_Extension）

> 文档状态：Draft v0.4  
> 维护者：PowerX CoreX 团队  
> 上次更新：2025-10-13

---

## 1. 目标与定位

- **轻量可演进**：在不引入复杂基础设施的前提下先落地图谱增益（Phase 1 用 PG 存节点/边，未来可切换图数据库驱动）。  
- **增强召回与推理**：把“实体—概念—文档—流程”的关系显式化，用于 **Hybrid 检索增广** 与 **Agent 多跳推理**。  
- **运营可控**：支持人工维护概念、别名与合并策略，提供审计与冲突检测。  

---

## 2. 图谱模型（Nodes & Edges）

### 2.1 节点类型

- `DocumentNode`：映射到 `KnowledgeDocument`（或细化到 `DocumentChunk`）。  
- `EntityNode`：抽取的业务实体（如产品、客户、组织、API 名称）。  
- `ConceptNode`：手工维护的业务概念或标签（如 “售后策略”）。  

### 2.2 边类型

- `reference`：文档到文档（含版本继承）。  
- `belong_to`：片段→文档、文档→空间（结构归属）。  
- `relate_to`：实体↔实体、实体↔概念、实体↔文档（通用关联）。  
- `workflow_anchor`：知识点（文档/实体/概念）与 Flow 节点的锚定。  

> 以上类型需在枚举表统一注册，避免自由字符串导致的不可控增长。

---

## 3. 存储方案与表结构

### 3.1 Phase 1：PostgreSQL 存储（推荐先落地）

- 元数据与关系落 PG，复用审计、多租户与事务特性。  
- 与 `knowledge_documents` / `document_chunks` 通过外键或业务主键关联。

#### `knowledge_entities`

```sql
id uuid PRIMARY KEY
tenant_id uuid NOT NULL
space_id uuid NOT NULL
type text            -- entity|concept|document
name text
alias jsonb          -- 别名集合
properties jsonb     -- 可扩展属性，如code、external_id
sensitivity text     -- low|normal|high|critical
created_at timestamptz
updated_at timestamptz
```

#### `knowledge_links`

```sql
id uuid PRIMARY KEY
tenant_id uuid NOT NULL
space_id uuid NOT NULL
from_id uuid NOT NULL    -- references knowledge_entities(id) or doc/chunk
to_id uuid NOT NULL
relation text            -- reference|belong_to|relate_to|workflow_anchor
weight numeric           -- [0,1] 推荐权重或置信度
provenance jsonb         -- 来源：parser/regex/llm/manual/plugin
sensitivity text         -- 继承或单独设定
created_at timestamptz
updated_at timestamptz
```

> 如需将 `DocumentChunk` 直接入图，可在 `knowledge_entities` 中为 chunk 建立虚拟节点（`type=chunk`），或以 `from_ref`/`to_ref` 记录业务主键（document_id+seq）。两种模式择一，建议**文档级为主、片段级按需开启**。

### 3.2 Phase 2：图数据库驱动（可选）

- 驱动接口 `GraphStore`：`UpsertNode/UpsertEdge/Neighbors/Path`。
- 实现：Neo4j / TigerGraph / JanusGraph / OpenCypher 兼容层。
- 与 PG 的**双写策略**：以 PG 为主数据，图库为查询加速层，可重建。

---

## 4. 构建与更新（Build & Update Pipeline）

### 4.1 自动抽取（解析阶段）

- **命名实体识别（NER）**：产品名、客户名、模块名、API 标识符。
- **模式规则/正则**：文档间引用（如 “见 §3.2”）、表格主键/外键映射。
- **LLM 辅助关系抽取**：用模板化 prompt 对关键段落抽三元组（带置信度）。
- 产出：`EntityNode`、`relate_to/reference/belong_to` 边，`provenance=parser|llm`。

### 4.2 人工维护（Admin 控制台）

- 概念树、别名合并、冲突处理（如“CRM”和“客户关系管理”同义）。
- 手工加边：把“标准规范文档”与“操作说明文档”建立 `reference`。
- Flow 节点锚定：维护 `workflow_anchor(node_id, flow_node_key)`。

### 4.3 插件同步（契约写入）

- 插件通过 SDK 写入实体与边：

  - CRM 插件：`客户`—`订单`（`relate_to`）、`客户`—`售后`（`relate_to`）。
  - 媒体库：`素材`—`使用场景`（`relate_to`）。
- 全量/增量同步受 `tenant_id/space_id` 限制，记录 `provenance=plugin`。

---

## 5. GraphResolver 服务

### 5.1 接口职责

- `Neighbors(node_id, relation?, depth=1, limit=K)`：邻接拓展。
- `Path(source_id, target_id, max_depth=3)`：限定跳数的路径发现。
- `Rank(nodes, query_ctx)`：基于关系与上下文的打分。
- `Suggest(query_ctx)`：返回与查询有关的**实体/概念候选**（用于自动补全与追问）。

### 5.2 关系打分（relation_score）

对候选节点（或其关联文档）计算图关系得分：

```
relation_score = Σ ( w_rel(type) * weight(edge) * decay(hops) * mask(sensitivity) )
```

- `w_rel(type)`：边类型权重（如 reference=0.6, relate_to=0.4, belong_to=0.2）。
- `weight(edge)`：边自身置信度（0~1），来源于抽取/人工维护/插件同步。
- `decay(hops)`：跳数衰减（如 `1/(1+hops)` 或 `exp(-λ*hops)`）。
- `mask(sensitivity)`：对高敏感级别节点做降权或屏蔽。

---

## 6. 与检索融合（Hybrid + Graph Boost）

将 `relation_score` 与检索得分融合，增强召回：

### 6.1 融合公式

在 `Retrieval_and_Ranking` 的 Stage-1 基础上，加入图谱增益：

```
score_final = score_base
            + γ * relation_score
```

- `score_base` 来自语义/关键词线性融合与业务加权；
- `γ = rank_profile.graph_weight`（空间级配置，默认 0.15）。

### 6.2 应用策略

- **实体触发**：当 Query 经 NER/规则识别出实体时，优先启用 graph boost。
- **路径限制**：默认仅使用 `depth<=2` 的贡献，控制运算量与风险。
- **MMR 之后融合**：先做去冗余，再应用图谱增益，避免“关系近亲”刷屏。

---

## 7. 与 Workflow / Agent 的结合

### 7.1 Workflow

- 在节点执行前，通过 `workflow_anchor` 找到相关知识点并注入上下文：

  ```yaml
  steps:
    - id: approve_refund
      type: human_task
      context:
        graph:
          anchor: "refund_policy"
          depth: 1
          limit: 10
  ```

- 典型用途：审批前自动拉取“政策规范、案例、风险提示”。

### 7.2 Agent

- Agent 先检索 Top-K，再调用 `GraphResolver.Neighbors` 拓展实体上下文，生成**追问建议**或**工具调用计划**。
- 在对话中可显示 `explain.graph` 字段：指出“为何命中该段落（来自某实体/概念的关系扩展）”。

---

## 8. 配置（config.yaml）

```yaml
graph:
  enabled: true
  store: "postgres"        # postgres | neo4j | tigergraph
  relation_weight:
    reference: 0.6
    relate_to: 0.4
    belong_to: 0.2
    workflow_anchor: 0.5
  decay:
    type: "exp"            # exp | inverse
    lambda: 0.6
  sensitivity:
    high_penalty: 0.3
    critical_penalty: 0.6

retrieval:
  hybrid:
    graph_weight: 0.15
    max_depth: 2
    graph_topk: 50
```

---

## 9. 事件与任务（Event & Jobs）

- `knowledge.graph.node.upserted`：节点新增或更新。
- `knowledge.graph.edge.upserted`：边新增或更新。
- `knowledge.graph.rebuild.requested`：触发重建（从 PG 主数据回放至图库）。
- 定时任务：

  - 别名合并与冲突检测（循环引用、重复命名）。
  - 低权重边清理（过期或置信度低于阈值）。

---

## 10. 监控与治理

- **指标**：

  - `kb_graph_nodes_total{type=entity|concept|document}`
  - `kb_graph_edges_total{relation=...}`
  - `kb_graph_build_latency_seconds`、`kb_graph_rebuild_jobs`
- **审计**：记录操作者、来源、改动摘要、影响范围（受影响文档/实体数）。
- **冲突检测**：

  - 别名冲突/循环引用/重复边；
  - 跨租户/跨空间违规关联（禁止或强制打低权重）。

---

## 11. 控制台（Admin）交互规范（摘要）

- **图谱总览**：按类型/关系过滤，显示节点度数与最近更新。
- **编辑模式**：点选节点，新增/删除边，设置权重与敏感级；
- **锚点管理**：将 `ConceptNode` 或 `EntityNode` 绑定到 Flow 节点；
- **解释视图**：在检索结果中显示图谱增益来源与路径片段。

---

## 12. 未来演进（Roadmap）

- **Phase 1**：文档—标签—实体轻图谱（PG 存储），与 Hybrid 检索融合。
- **Phase 2**：实体抽取增强（LLM+规则），图数据库驱动接入与双写回放。
- **Phase 3**：Streaming 事件实时增量、路径推理 API、多跳问答优化。
