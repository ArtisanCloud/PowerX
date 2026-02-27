# API 与 SDK 设计（API_and_SDK_Design）

> 文档状态：Final v1.1  
> 维护者：PowerX CoreX 团队  
> 上次更新：2025-10-13

---

## 1. 目标与原则

- **统一契约**：仅提供 HTTP 与 gRPC 接口；不发布独立 SDK。
- **多租户与安全**：强制 `JWT claims（tid/tenant_uuid）`，RBAC 最小权限，完整审计。
- **可演进**：兼容 Rank Profile、Graph Boost、A/B/灰度、回放与回滚。
- **工程完备**：分页、过滤、幂等、速率限制、错误码、可观测。

---

## 2. 基础约定

### 2.1 路径与版本（以下均为完整路径）

- REST Base 前缀：`/api/v1/knowledge/...`
- gRPC Package：`knowledge.v1`
- 返回骨架（统一）：

  ```json
  { "code": 0, "message": "ok", "data": { }, "trace_id": "..." }

````

### 2.2 认证与权限

* 认证：`Authorization: Bearer <token>`
* 租户：`JWT claims（tid/tenant_uuid）: <uuid>`（必填）
* 权限（示例）：

  * `knowledge:space` → `create|read|update|delete|admin`
  * `knowledge:document` → `create|read|update|delete|publish`
  * `knowledge:search` → `search`
  * `knowledge:graph` → `read|update`

### 2.3 常用查询/分页/过滤

* `page`（默认 1） / `page_size`（默认 20，最大 200）
* `q`（模糊） / `updated_after` / `updated_before`（ISO8601）
* 过滤：`space_id`、`tags[]`、`source_types[]`、`sensitivity_max`

### 2.4 幂等与重试

* 写接口支持 `Idempotency-Key`（<tenant,path,key> 24h 去重）
* 客户端重试仅限 GET/幂等 POST，指数退避；遵循 429/5xx 语义

### 2.5 速率限制（默认）

* 管理类：60 RPM；检索类：600 RPM（并发 ≤ 20）
* 超限：HTTP `429` + `Retry-After`；gRPC `RESOURCE_EXHAUSTED`

### 2.6 错误码（摘录）

* `400xxx` 参数非法 / 状态不符
* `401xxx` 未认证；`403xxx` 权限不足
* `404xxx` 资源不存在；`409xxx` 冲突
* `422xxx` 语义错误；`429xxx` 速率限制
* `5xx` 内部错误或外部依赖失败

---

## 3. REST API（完整路径）

> 所有接口均要求：`Authorization`（租户由 JWT claims 提供）。
> 响应统一包裹于 `{"code","message","data","trace_id"}`。

### 3.1 空间（Spaces）

| 方法       | 路径                                   | 描述                                                    |
| -------- | ------------------------------------ | ----------------------------------------------------- |
| `POST`   | `/api/v1/knowledge/spaces`           | 创建空间（可内含默认 Rank Profile 与 Chunker 配置）                 |
| `GET`    | `/api/v1/knowledge/spaces`           | 列表/过滤                                                 |
| `GET`    | `/api/v1/knowledge/spaces/{spaceID}` | 详情（含文档数、索引状态摘要）                                       |
| `PATCH`  | `/api/v1/knowledge/spaces/{spaceID}` | 更新设置（`rank_profile`、`settings.chunker`、`retrieval` 等） |
| `DELETE` | `/api/v1/knowledge/spaces/{spaceID}` | 软删除                                                   |

**请求体（创建）**

```json
{
  "name": "crm_docs",
  "slug": "crm_docs",
  "visibility": "private",
  "rank_profile": {
    "semantic_weight": 0.65,
    "recency_weight": 0.02,
    "normalize": "minmax",
    "mmr_beta": 0.7,
    "graph_weight": 0.15,
    "rerank": { "enabled": true, "topk": 20, "provider": "cross-encoder/ms-marco-MiniLM-L-6-v2" }
  },
  "settings": { "chunker": { "window": 512, "overlap": 64 } }
}
```

### 3.2 来源（Sources）

| 方法     | 路径                                           | 描述                        |
| ------ | -------------------------------------------- | ------------------------- |
| `POST` | `/api/v1/knowledge/spaces/{spaceID}/sources` | 注册来源（文件/URL/Webhook/插件）   |
| `GET`  | `/api/v1/knowledge/spaces/{spaceID}/sources` | 列表 + 同步状态                 |
| `POST` | `/api/v1/knowledge/sources/{sourceID}:sync`  | 触发同步（manual/cron/webhook） |

**来源定义（示例）**

```json
{
  "type": "webhook|file|url|plugin",
  "config": { "endpoint":"...", "secret":"..." },
  "tags": ["policy","faq"]
}
```

### 3.3 文档与版本（Documents & Versions）

| 方法       | 路径                                                  | 描述                        |
| -------- | --------------------------------------------------- | ------------------------- |
| `POST`   | `/api/v1/knowledge/documents`                       | 创建/上传（`multipart` 或 JSON） |
| `GET`    | `/api/v1/knowledge/documents`                       | 列表/过滤                     |
| `GET`    | `/api/v1/knowledge/documents/{documentID}`          | 详情 + 版本列表                 |
| `POST`   | `/api/v1/knowledge/documents/{documentID}/versions` | 新版本（触发解析与索引）              |
| `POST`   | `/api/v1/knowledge/documents/{documentID}/publish`  | 发布可检索                     |
| `DELETE` | `/api/v1/knowledge/documents/{documentID}`          | 软删除（触发向量清理）               |

**`multipart/form-data` 字段**

* `file`: 二进制文件（如 `application/pdf`）
* `metadata`: JSON（`space_id`, `title`, `tags[]`, `sensitivity`, `attributes`）

### 3.4 检索（Search）

| 方法     | 路径                               | 描述                           |
| ------ | -------------------------------- | ---------------------------- |
| `GET`  | `/api/v1/knowledge/search`       | Hybrid 检索；支持解释字段与 Profile 覆盖 |
| `POST` | `/api/v1/knowledge/search:batch` | 批量查询（Agent/Workflow 批处理）     |

**查询参数**

```
query: string                      # 查询文本
space_id: string                   # 空间
k: int=8                           # Top-N
filters.tags[]?                    # 标签过滤
filters.source_types[]?            # 来源类型过滤
filters.sensitivity_max?           # normal|high|critical
context.conversation_id?           # 调试/审计
context.workflow_run_id?           # 调试/审计
rank_profile_override? (JSON)      # 可选覆盖（临时策略）
```

**返回（简化）**

```json
{
  "code":0,"message":"ok",
  "data": {
    "query":"媒体存储如何配置","top_n":8,
    "items":[
      {
        "chunk_id":"c_123","document_id":"d_456","version_no":3,
        "text_snippet":"……","highlights":["媒体","存储"],
        "score":0.842,
        "scores":{"semantic":0.91,"keyword":0.62,"recency":0.05,"source_boost":0.30,"sensitivity_penalty":0.00,"feedback":0.02,"rerank":0.12,"graph":0.08},
        "explain":"语义为主 + 来源加权 + 近期更新 + 图谱增益",
        "source_type":"kb_spec","space_id":"crm_docs","tags":["media","config"]
      }
    ]
  },
  "trace_id":"..."
}
```

### 3.5 反馈（Feedback）

| 方法     | 路径                           | 描述             |
| ------ | ---------------------------- | -------------- |
| `POST` | `/api/v1/knowledge/feedback` | 上报评分/点击/无帮助等信号 |

**请求体**

```json
{ "query_id":"q_20251013_abc", "chunk_id":"c_123", "rating":1, "comment":"命中准确", "user_id":"u_88" }
```

### 3.6 图谱（Graph）

| 方法     | 路径                                  | 描述                 |
| ------ | ----------------------------------- | ------------------ |
| `GET`  | `/api/v1/knowledge/graph/nodes`     | 查询节点（实体/概念/文档）     |
| `GET`  | `/api/v1/knowledge/graph/neighbors` | 邻居拓展               |
| `GET`  | `/api/v1/knowledge/graph/paths`     | 限跳路径发现             |
| `POST` | `/api/v1/knowledge/graph/anchors`   | 维护 Workflow 锚点（可选） |

**示例：neighbors 参数**

```
node_id: string
depth: int=1
limit: int=50
types[]?: entity|concept|document
```

---

## 4. gRPC 契约（接口原型）

> 插件/服务侧请按以下 proto 使用（**仅定义，按你现有“gRPC 实现规范”落地**）。

```proto
syntax = "proto3";
package knowledge.v1;

// 认证与多租户在网关或拦截器中处理：authorization/x-tenant-uuid

service KnowledgeService {
  // Spaces
  rpc CreateSpace(CreateSpaceRequest) returns (Space);
  rpc ListSpaces(ListSpacesRequest) returns (ListSpacesResponse);
  rpc GetSpace(GetSpaceRequest) returns (Space);
  rpc UpdateSpace(UpdateSpaceRequest) returns (Space);
  rpc DeleteSpace(DeleteSpaceRequest) returns (Ack);

  // Documents & Versions
  rpc UpsertDocument(UpsertDocumentRequest) returns (Document);
  rpc ListDocuments(ListDocumentsRequest) returns (ListDocumentsResponse);
  rpc GetDocument(GetDocumentRequest) returns (Document);
  rpc CreateVersion(CreateVersionRequest) returns (DocumentVersion);
  rpc PublishDocument(PublishDocumentRequest) returns (Ack);
  rpc DeleteDocument(DeleteDocumentRequest) returns (Ack);

  // Search
  rpc Search(KBQueryRequest) returns (KBQueryResponse);
  rpc BatchSearch(BatchKBQueryRequest) returns (BatchKBQueryResponse);

  // Feedback
  rpc SubmitFeedback(SubmitFeedbackRequest) returns (Ack);

  // Graph
  rpc GraphNodes(GraphNodesRequest) returns (GraphNodesResponse);
  rpc GraphNeighbors(GraphNeighborsRequest) returns (GraphNeighborsResponse);
  rpc GraphPaths(GraphPathsRequest) returns (GraphPathsResponse);

  // Streaming（可选）
  rpc StreamIngest(stream IngestChunk) returns (IngestAck);
  rpc SearchStream(KBQueryRequest) returns (stream KBQueryChunk);
}

// ====== Messages（摘要） ======
message Space { string id=1; string name=2; string slug=3; string visibility=4; map<string,string> labels=5; }
message CreateSpaceRequest { string name=1; string slug=2; string visibility=3; map<string,string> labels=4; bytes rank_profile=10; bytes settings=11; }
message ListSpacesRequest { int32 page=1; int32 page_size=2; string q=3; }
message ListSpacesResponse { repeated Space items=1; int32 page=2; int32 page_size=3; int32 total=4; }
message GetSpaceRequest { string space_id=1; }
message UpdateSpaceRequest { string space_id=1; bytes rank_profile=10; bytes settings=11; }
message DeleteSpaceRequest { string space_id=1; }

message Document { string id=1; string space_id=2; string title=3; string status=4; int32 version_no=5; }
message UpsertDocumentRequest { string space_id=1; string title=2; repeated string tags=3; string sensitivity=4; bytes attributes=5; /* 上传走 HTTP，gRPC 走 StreamIngest */ }
message CreateVersionRequest { string document_id=1; string note=2; }
message PublishDocumentRequest { string document_id=1; }
message DeleteDocumentRequest { string document_id=1; }

message KBQueryRequest {
  string space_id=1; string query=2; int32 k=3;
  bytes filters=10; bytes context=11; bytes rank_profile_override=12;
}
message KBQueryItem {
  string chunk_id=1; string document_id=2; int32 version_no=3;
  string text_snippet=4; repeated string highlights=5;
  double score=6; map<string,double> scores=7; string explain=8;
  string source_type=9; string space_id=10; repeated string tags=11;
}
message KBQueryResponse { string query=1; int32 top_n=2; repeated KBQueryItem items=3; }

// Graph（摘要）
message GraphNodesRequest { string q=1; string type=2; int32 page=3; int32 page_size=4; }
message GraphNode { string id=1; string type=2; string name=3; bytes properties=4; }
message GraphNodesResponse { repeated GraphNode items=1; int32 page=2; int32 page_size=3; int32 total=4; }

message GraphNeighborsRequest { string node_id=1; int32 depth=2; int32 limit=3; repeated string types=4; }
message GraphNeighborsResponse { GraphNode center=1; repeated GraphNode neighbors=2; }

message GraphPathsRequest { string src_id=1; string dst_id=2; int32 max_depth=3; }
message GraphPathsResponse { repeated GraphPath paths=1; }
message GraphPath { repeated GraphNode nodes=1; }

message SubmitFeedbackRequest { string query_id=1; string chunk_id=2; int32 rating=3; string comment=4; string user_id=5; }
message Ack { bool ok=1; string message=2; }

// Streaming（上传/检索）
message IngestChunk { string space_id=1; string document_id=2; bytes payload=3; string content_type=4; }
message IngestAck { bool ok=1; string document_id=2; }
message KBQueryChunk { KBQueryItem item=1; }
```

> 说明：`bytes` 字段用于承载 JSON（RankProfile/Filters/Settings 等）以保持前后兼容；Streaming 接口供大文件/日志与流式检索使用。

---

## 5. OpenAPI 片段（示例）

```yaml
openapi: 3.0.3
info: { title: PowerX Knowledge API, version: "1.0" }
servers: [ { url: / } ]
paths:
  /api/v1/knowledge/search:
    get:
      summary: Hybrid search with explain fields
      parameters:
        - in: header
          name: Authorization
          required: true
          schema: { type: string }
        - in: header
          name: JWT claims（tid/tenant_uuid）
          required: true
          schema: { type: string, format: uuid }
        - in: query
          name: query
          required: true
          schema: { type: string }
        - in: query
          name: space_id
          required: true
          schema: { type: string }
        - in: query
          name: k
          schema: { type: integer, default: 8, maximum: 50 }
      responses:
        "200":
          description: OK
          content:
            application/json:
              schema: { $ref: "#/components/schemas/SearchEnvelope" }

  /api/v1/knowledge/graph/neighbors:
    get:
      summary: Graph neighbors expansion
      parameters:
        - in: header
          name: Authorization
          required: true
          schema: { type: string }
        - in: header
          name: JWT claims（tid/tenant_uuid）
          required: true
          schema: { type: string, format: uuid }
        - in: query
          name: node_id
          required: true
          schema: { type: string }
        - in: query
          name: depth
          schema: { type: integer, default: 1, maximum: 3 }
        - in: query
          name: limit
          schema: { type: integer, default: 50, maximum: 200 }
      responses:
        "200":
          description: OK
          content:
            application/json:
              schema: { $ref: "#/components/schemas/NeighborsEnvelope" }

components:
  schemas:
    Envelope:
      type: object
      properties: { code: {type: integer}, message: {type: string}, data: { }, trace_id: {type: string} }

    SearchEnvelope:
      allOf:
        - $ref: "#/components/schemas/Envelope"
        - type: object
          properties:
            data:
              type: object
              properties:
                query: { type: string }
                top_n: { type: integer }
                items:
                  type: array
                  items: { $ref: "#/components/schemas/SearchItem" }

    SearchItem:
      type: object
      properties:
        chunk_id: { type: string }
        document_id: { type: string }
        version_no: { type: integer }
        text_snippet: { type: string }
        highlights: { type: array, items: { type: string } }
        score: { type: number, format: float }
        scores: { type: object, additionalProperties: { type: number } }
        explain: { type: string }
        source_type: { type: string }
        space_id: { type: string }
        tags: { type: array, items: { type: string } }

    NeighborsEnvelope:
      allOf:
        - $ref: "#/components/schemas/Envelope"
        - type: object
          properties:
            data:
              type: object
              properties:
                center: { $ref: "#/components/schemas/GraphNode" }
                neighbors:
                  type: array
                  items: { $ref: "#/components/schemas/GraphNode" }

    GraphNode:
      type: object
      properties:
        id: { type: string }
        type: { type: string, enum: [entity, concept, document] }
        name: { type: string }
        properties: { type: object, additionalProperties: true }
```

---

## 6. 事件与 Webhook

### 6.1 事件主题（Event Bus）

* `knowledge.space.created`
* `knowledge.document.versioned`
* `knowledge.document.indexed`
* `knowledge.indexing.failed`
* `knowledge.feedback.submitted`
* `knowledge.graph.node.upserted` / `knowledge.graph.edge.upserted`

### 6.2 Webhook（可选）

* 注册：`/api/v1/knowledge/spaces/{spaceID}/webhooks`
* 签名：`X-PowerX-Signature: sha256=<digest>`
* 重试：指数退避 3 次 → DLQ

---

## 7. 客户端接入（无 SDK）

### 7.1 HTTP（PowerX Web Admin）

* 请求头：`Authorization`, `JWT claims（tid/tenant_uuid）`
* 建议超时：检索 3–5s；上传 30–120s（大文件）
* 幂等：创建/更新类可加 `Idempotency-Key`

**最小调用（示例）**

```
GET /api/v1/knowledge/search?space_id=crm_docs&query=媒体存储如何配置&k=8
Headers:
  Authorization: Bearer <token>
  JWT claims（tid/tenant_uuid）: <uuid>
```

### 7.2 gRPC（插件/服务侧）

* 代码按第 4 章 proto 生成；拦截器统一注入 `authorization` 与 `x-tenant-uuid` 元数据
* 建议超时：检索 3–5s；Rerank 开启可至 6–8s
* 重试：仅对 `UNAVAILABLE` / `DEADLINE_EXCEEDED` 等幂等场景

---

## 8. 版本与兼容

* 主线版本：`v1`；重大变更发布 `v2`，并提供 ≥ 6 个月过渡
* 向后兼容优先：新增可选字段；删除/替换需跨版本公告与映射
* Profile 与策略版本化：可灰度、回滚与查询回放

---

## 9. 安全与合规

* `sensitivity_max` 过滤 + 片段级脱敏（最小引用单元）
* 全量审计：操作者/IP/摘要/差异；`trace_id` 串联链路
* 返回 `document_id + version_no`，便于溯源与回放
