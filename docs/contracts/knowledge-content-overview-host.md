# 已发布知识内容导览 Host 合同

版本：`powerx.knowledge.content-overview/v1`，2026-10-10。
正式类型：[OpenAPI](../../specs/011-knowledge-space/contracts/content-overview.openapi.yaml)。

## 1. 用途和支持范围

读取一个知识空间在指定检索模式下的完整文章目录，逐篇返回标题及已发布原文片段预览。目录查询不调用 Embedding 或 LLM，也不重新入库。

- v1 支持 `semantic`、`hybrid` 和 PostgreSQL/pgvector；不把旧 lexical 文档列作语义文章。
- 一篇文章对应一个稳定的 Core `document_uuid`。只列当前 active 成功任务、当前配置 generation、`indexed` 且 `queryable=true` 的文档；需要真实向量行及对应 `source_chunk`。仅有画像的记录不计入文章目录。
- 目录与语义检索共用 `PublishedSemanticFrom` 的租户、空间、任务发布、软删除和可见性条件。`semantic` 包含已发布 hybrid 文章，`hybrid` 仅包含 hybrid 文章。
- 标题、URI 和文档版本来自成功任务冻结的 source snapshot；不会读取尚未发布的新草稿标题或内容。
- 预览取每篇首个已发布 source chunk 的前 N 个 Unicode codepoint。`preview.kind=published_source_chunk`；它不是 AI 摘要，也不是整篇原文件下载。`truncated` 表示该片段被预览长度截断。整篇原文件读取需要另行授权及接口。
- `source_ref` 保留原文 UUID/version/checksum 和片段范围；`preview.char_start/char_end` 是原文中的预览范围，结束位置不包含在内。中文、emoji 均按 codepoint 计数。
- 没有概要/主题/推荐问题产物：三个字段均明确为 `not_available / KNOWLEDGE_CONTENT_DERIVED_NOT_PRODUCED`。不得从预览推断平台已生成这些产物。

## 2. 授权

正式 capability：`com.corex.knowledge.content_overview.service_read`。

| 平面 | 凭证及检查 |
|---|---|
| Framework delegated / Local Host simulation | `core_internal` + `service_actor`，固定 `INVOKE core://knowledge/content-overview`；API Key 或 STS |
| Tenant REST | `Authorization: ApiKey <key>` 或 `Bearer <STS>`；同一 capability 的 published record + tenant registration + 当前凭据实时授权 |
| Admin | 用户 JWT + 当前租户 RBAC；permission_code `corex.knowledge.content_overview:read` 对应正式 IAM 三元组 `knowledge_space / knowledge_content_overview / read`、source `platform_capability`；拒绝 API Key/STS 服务身份 |

API Key 精确 grant：scope `_scope.knowledge.content_overview.service_read`、action `read`、resource_type `api`、resource_pattern `content_overview`、effect `allow`。IAM service permission 必须同时有 `allow_api_key=true`、`meta.api_key_explicit=true`。Admin REST permission 不能代替此 grant。调用 `capabilities:grant-status` 自查还需独立的 `com.corex.capabilities.grant_status.read`（scope `_scope.capabilities.grant_status.read`、read/api/`grant-status`）；它不扩大文章读取范围。

STS 必须由正式 Exchange 签发：issuer `powerx-sts`，audience `powerx:api`，scope `access`；同租户启用的插件 `auth.credentials.allowed_capabilities` 包含此 capability，过期或撤销即时拒绝。

此版本的授权范围明确为 `current_credential_tenant_published_corpus`：租户服务凭据的授权语料，以及 Core 文档当前可见性。它不承诺按浏览器终端用户身份代授权；当前 retrieval 也没有此类用户级文章 ACL。插件必须先检查自己的用户与空间访问权限。若后续新增 Core 对象 ACL，须同时扩展共享 corpus 条件及版本/游标失效规则。

现有 `knowledge.directory.read` 是空间目录权限，不授权枚举文章正文。无需授予 `document.manage` 或宽泛 Admin manage。

## 3. REST 与 typed 操作

```http
GET /api/v1/tenant/knowledge/spaces/{space_uuid}/content-overview?mode=hybrid&page_size=20&preview_codepoints=320
Authorization: ApiKey <redacted>
X-Trace-ID: <request-trace>
```

Admin 路径：`GET /api/v1/admin/knowledge-spaces/{spaceId}/content-overview`，使用同一查询字段。

| 字段 | 规则 |
|---|---|
| `mode` | 必填，semantic / hybrid；空间未开放该模式返回 422 |
| `required_corpus_generation` | 可选 UUID；不匹配返回 409，不能静默改用最新语料 |
| `page_size` | 默认 20，1–100 |
| `preview_codepoints` | 默认 320，0–2000；显式 0 返回空预览文字，仍保留来源 |
| `cursor` | 不透明签名游标，最长 4096 字符 |

未知字段、重复查询字段、租户覆盖和非法值返回 400。

Framework 通过正式 tenant invocation 调用 typed binding：

```json
{
  "capability_id": "com.corex.knowledge.content_overview.service_read",
  "preferred_protocol": "core_internal",
  "payload": {
    "body": {
      "operation": "list_articles",
      "query": {
        "schema": "powerx.knowledge.content-overview-query/v1",
        "space_uuid": "11111111-1111-4111-8111-111111111111",
        "mode": "hybrid",
        "page_size": 20,
        "preview_codepoints": 320
      }
    }
  }
}
```

请求发到 `POST /api/v1/tenant/invocations`。binding 的内部结果为 `{ "result": <ContentOverview> }`，外层保留 Registry 的 invocation envelope。不得指定任意 HTTP method、endpoint、headers、query 或 raw proxy；不支持的操作明确失败。

## 4. 响应和分页

成功返回标准 Core envelope：`code/message/data/timestamp/request_id`。`data` 示例（示例 UUID，不是验收记录）：

```json
{
  "schema": "powerx.knowledge.content-overview/v1",
  "space_uuid": "11111111-1111-4111-8111-111111111111",
  "mode": "hybrid",
  "authorization_scope": "current_credential_tenant_published_corpus",
  "generation": {
    "env": "dev",
    "space_uuid": "11111111-1111-4111-8111-111111111111",
    "configuration_generation": "22222222-2222-4222-8222-222222222222",
    "corpus_generation": "33333333-3333-4333-8333-333333333333",
    "embedding_profile": {"uuid":"44444444-4444-4444-8444-444444444444","version":1},
    "model_key":"ollama/bge-m3",
    "model_revision":"<frozen-model-revision>",
    "dimensions":1024,
    "distance_metric":"cosine"
  },
  "items": [{
    "document_uuid":"55555555-5555-4555-8555-555555555555",
    "title":"产品使用说明",
    "uri":"powerx://documents/product-guide",
    "document_version":"v1",
    "bundle_generation":"66666666-6666-4666-8666-666666666666",
    "chunk_uuid":"77777777-7777-4777-8777-777777777777",
    "artifact_uuid":"88888888-8888-4888-8888-888888888888",
    "source_ref":{"kind":"cleaned_document","source_uuid":"99999999-9999-4999-8999-999999999999","version":"v1","checksum":"<actual-sha256>","position_unit":"unicode_codepoint","char_start":0,"char_end":640},
    "preview":{"kind":"published_source_chunk","text":"产品的实际原文片段……","position_unit":"unicode_codepoint","char_start":0,"char_end":11,"truncated":true}
  }],
  "pagination":{"page_size":20,"total":1,"has_more":false,"next_cursor":null},
  "overview":{"status":"not_available","reason_code":"KNOWLEDGE_CONTENT_DERIVED_NOT_PRODUCED"},
  "themes":{"status":"not_available","reason_code":"KNOWLEDGE_CONTENT_DERIVED_NOT_PRODUCED"},
  "recommended_questions":{"status":"not_available","reason_code":"KNOWLEDGE_CONTENT_DERIVED_NOT_PRODUCED"},
  "trace_id":"<request-trace>"
}
```

`bundle_generation` 是该文档当前成功索引任务 UUID；`generation.corpus_generation` 是空间语料版本，二者不能互换。`total` 是当前授权及模式过滤后的文章数，无文章合法返回 `items=[]/total=0/has_more=false/next_cursor=null`。

按文档 UUID 稳定排序。游标绑定凭据、租户、空间、模式、页大小、预览长度、语料 generation、发布任务及 visibility epoch 的目录指纹；固定有效期 15 分钟。服务端完成来源校验后再复查空间、发布状态、可见性和授权。

- 延续游标必须保持所有参数。不同凭据不能复用游标。
- 语料/配置变化、目录或 visibility 变化返回 409；授权撤销返回 403。
- 服务进程重启使用相同 `server.secret_key` 时游标仍有效；轮换该密钥使旧签名失效。
- 不能在 409 后拼接旧页和新页。消费端清空目录，从新 generation 第一页重新读取。

## 5. 稳定错误

| HTTP | 机器码 / 语义 |
|---|---|
| 400 | `KNOWLEDGE_INVALID_ARGUMENT`、`KNOWLEDGE_CONTENT_INVALID_ARGUMENT`、`KNOWLEDGE_CONTENT_CURSOR_INVALID`、`KNOWLEDGE_CONTENT_CURSOR_SCOPE_MISMATCH` |
| 401 | `KNOWLEDGE_UNAUTHORIZED`；认证中间件可先返回 `IAM_UNAUTHORIZED` |
| 403 | `KNOWLEDGE_FORBIDDEN`；无正式授权、注册不可用或实时撤销 |
| 404 | `KNOWLEDGE_SPACE_NOT_FOUND`；包括跨租户、已退休或已删除空间 |
| 409 | `KNOWLEDGE_INDEX_GENERATION_CONFLICT`、`KNOWLEDGE_CONTENT_SNAPSHOT_CONFLICT`、`KNOWLEDGE_CONTENT_CURSOR_EXPIRED`、`KNOWLEDGE_MODEL_BINDING_CONFLICT` |
| 422 | `KNOWLEDGE_SEMANTIC_INDEX_NOT_READY`、`KNOWLEDGE_SEMANTIC_MODE_NOT_READY`、`KNOWLEDGE_VECTOR_LAYOUT_INVALID` |
| 501 | `KNOWLEDGE_SEMANTIC_UNSUPPORTED` |
| 503 | `KNOWLEDGE_CONTENT_UNAVAILABLE`、`KNOWLEDGE_RESULT_HYDRATION_FAILED` 或既有 dependency 错误 |

保留 HTTP 状态、错误 envelope 的机器码、`request_id` 与 `X-Trace-ID`。失败不能映射成空成功目录。

## 6. 交付与验收

实现：`content_overview_service.go`、`published_corpus_repository.go`、Tenant/Admin Handler、typed invoker、正式 capability YAML。无新增数据表或迁移。

验收及未完成边界见 [实施记录](../../specs/011-knowledge-space/content-overview-implementation.md)。Framework 接入见 [对齐清单](knowledge-content-overview-framework-alignment.md)。
