# Core 通用语义索引与检索合同

日期：2026-10-08。范围：PowerX Core；Framework、插件代码由消费端对齐。

正式定义：[OpenAPI](../../specs/011-knowledge-space/contracts/semantic-host.openapi.yaml)。真实 API/Trace 证据：[验收记录](knowledge-semantic-acceptance-20261008.json)。实现任务：[Spec 补充](../../specs/011-knowledge-space/semantic-host-implementation.md)。Framework 接入：[对齐清单](knowledge-semantic-framework-alignment.md)。

## 1. 职责与支持范围

Core 执行完整自然语言的实际 Embedding、向量入库、严格检索、前置过滤、原文/画像来源回读和一致版本发布。知识画像的业务维度、提取、人工修订、客户适配分由插件负责。`knowledge_profile` 是插件业务画像，和 Core 的 Ingestion/RAG/Embedding Profile 是不同对象。

当前正式语义驱动为 **pgvector，与 Core 业务记录位于同一个 PostgreSQL 实例/数据库**。其他驱动、独立向量数据库明确返回 501；不会执行 hash/noop 或改用旧 LIKE 接口。实际模型由租户 AI Embedding 配置解析；Ollama 修订取 `/api/tags` 的真实 digest，其他 Provider 必须配置明确的 `model_revision`。已用 Ollama `bge-m3`、`bge-large` 验收，实际维度以探针和执行结果为准。

文本快照支持 TXT/Markdown，来源位置为 Unicode codepoint 半开区间。提交 artifact 必须包含完整不可变文本，`char_start=0`，`char_end=文本 codepoint 数`；Core 返回分块的准确子区间。画像定位自己的画像快照，不伪造原文位置。分页/bbox、任意过滤表达式、任意 metadata、业务维度比较、rerank、调用方自选查询模型不属于此 DTO，未知字段报错。分段设置沿用文档入库合同，无法准确映射来源的转换显式失败 `KNOWLEDGE_SOURCE_POSITION_UNAVAILABLE`。

## 2. 入口与授权

| 操作 | 服务态 REST | capability / typed operation |
| --- | --- | --- |
| 能力与当前/待发布模型 | GET `/api/v1/tenant/knowledge/spaces/{space_uuid}/semantic-index` | `com.corex.knowledge.retrieval.read` / `capabilities` |
| 模型配置冻结 | POST 同上 | `com.corex.knowledge.document.manage` / `configure_semantic_index` |
| 提交文档/画像 bundle | POST `/api/v1/tenant/knowledge/spaces/{space_uuid}/documents` | document.manage / `submit_document` |
| 检索 | POST `/api/v1/tenant/knowledge/retrieval/query` | retrieval.read / `query` |
| 单篇重建 | POST `/api/v1/tenant/knowledge/spaces/{space_uuid}/documents/{document_uuid}/indexes:rebuild` | document.manage / `rebuild_document` |
| 空间重建 | POST `/api/v1/tenant/knowledge/spaces/{space_uuid}/indexes:rebuild` | document.manage / `rebuild_space` |
| 任务 / 实际分块 | GET `/api/v1/tenant/knowledge/index-jobs/{job_uuid}`，末尾加 `/chunks` | document.manage / `get_index_job`、`get_index_job_chunks` |
| 可查询资格 | PATCH `/api/v1/tenant/knowledge/spaces/{space_uuid}/documents/{document_uuid}/visibility` | document.manage / `set_document_visibility` |

服务凭据为有效 API Key 或 STS；授权同时检查已发布 capability、tenant registration、实时凭据 grant。API Key 精确权限：

- 检索：scope `_scope.knowledge.retrieval.read`、action `read`、resource_type `api`、resource_pattern `retrieval`。
- 写入/重建：scope `_scope.knowledge.document.manage`、action `manage`、resource_type `api`、resource_pattern `document`。
- 两者来自正式 `core_internal` binding 的显式 `api_key` 元数据，IAM permission 具有 `allow_api_key=true`、`api_key_explicit=true`。
- 这是 tenant 范围文档授权；不存在插件任意传 tenant/plugin identity 覆盖可信身份的接口。生产凭据须由管理员单独授予；本次仅为既有开发 Host key 补齐精确 retrieval.read grant，保留其其他权限。

Admin 入口对应 `/api/v1/admin/knowledge-spaces/retrieval/query`、`/{spaceId}/semantic-index`、`/{spaceId}/documents/{documentId}/visibility`。必须用户 JWT + tenant/member + RBAC；读取为 `corex / knowledge.retrieval / read`，写入为 `corex / knowledge.document / manage`。STS 不能调用 Admin 平面。

正式 typed binding 使用 `INVOKE core://knowledge/retrieval` 或 `INVOKE core://knowledge/documents`，调用 `/api/v1/tenant/invocations` 时 **显式提供 `preferred_protocol: core_internal`**。不接收任意 HTTP method、endpoint、headers、query 或 raw proxy 参数。

```json
{
  "capability_id": "com.corex.knowledge.retrieval.read",
  "preferred_protocol": "core_internal",
  "payload": {
    "body": {
      "operation": "query",
      "query": {
        "schema": "powerx.knowledge.retrieval-query/v1",
        "query": "网上出现很多负面讨论，希望参考能回应公众质疑并恢复信誉的办法",
        "space_uuids": ["11111111-1111-4111-8111-111111111111"],
        "mode": "hybrid",
        "top_k": 20,
        "filters": {
          "document_uuids": [],
          "category_codes": ["public_relations"],
          "tag_uuids": [],
          "artifact_roles": ["knowledge_profile", "source_chunk"]
        }
      }
    }
  }
}
```

REST 成功 envelope 为 `{code,message,data,timestamp,request_id}`；typed invocation 的结果在 `data.result` / `data.payload`，其业务内容为 `{result: SemanticResult}`。`capabilities` operation 返回 `{capabilities: ...}`。实际完整响应见验收 JSON；示例 UUID 需要替换成当前空间/配置返回值。

## 3. 配置、模型切换与发布

1. 空间已有真实 Embedding Profile key 和激活的 dense index。管理员可通过现有 POST `/api/v1/admin/knowledge-spaces/{spaceId}/vector-index/activate` 选择模型及实际探测维度。
2. POST `semantic-index` 提交 `embedding_profile_key`，可携带 `expected_configuration_generation` 防止配置冲突。返回不可变 Embedding Profile UUID/version、模型 digest、维度、configuration_generation 和 corpus_generation。
3. 首次配置直接成为当前配置；再次换模型/维度/配置时，Core 保存 **pending_configuration**，能力接口返回 `pending_generation`。当前查询仍使用旧 binding 的模型及固定物理向量表；预备模型不会提前隐藏旧索引。
4. 用待发布的 Profile UUID/version 显式提交 `apply_config` 空间重建。所有原文/画像完成 Embedding、向量落库回读和来源验证后，任务、所有文档 active pointer、模型 binding 和 corpus generation 在同一事务中切换。
5. 存在其他旧模型文档时，单篇任务不能发布整个空间的新模型，会明确失败 `KNOWLEDGE_MODEL_SPACE_REBUILD_REQUIRED`；不会扩大成空间重建。任务失败保留旧一致版本。
6. 外部模型本身失效或 alias 的实际 digest 变化，严格返回模型不可用/修订冲突；不会以旧索引和新模型混算。旧数据保留不代表失效的模型还能执行查询。

冻结配置同时保存 `env`；后台 Worker 使用配置环境解析模型，不会把生产任务误交给 dev 模型。

`configuration_generation` 表示模型/索引配置；`corpus_generation` 表示当前发布/可见性代次；`bundle_generation` 使用成功任务 UUID。它们不能合并成一个版本号。

## 4. 文档 bundle 与任务

在原文提交 DTO 上新增：

- `indexing: {mode: semantic|hybrid, embedding_profile: {uuid,version}, artifact_roles: [...]}`。
- `external_ref: {document_uuid}`：插件文档 UUID。Core document UUID 独立生成且稳定；同空间同外部 UUID 不允许悄悄改 URI 映射，同 URI 不允许更换已绑定的外部 UUID。
- `artifacts[]`：UUID、role、text、checksum、version、source_ref；可附 `knowledge_profile_uuid`、`case_uuid`、`category_codes[]`、`tag_uuids[]`。`knowledge_profile` 必须关联其自己的 profile UUID。
- `idempotency_key`：同键同请求返回原任务，同键不同请求 409；模型/Profile 后续变化不会使原已受理请求的幂等回读失效。正在执行的冲突任务 409；已成功的相同来源/配置不能换一个 key 再重复执行。失败重试用新 key。
- `ingestion`：原正式入库配置，显式 false/0/空数组/顺序保留；如 `chunk_size=640`、`chunk_overlap=80`。

提交完整示例（embedding_profile 必须替换为 Configure 返回的真实 UUID/version；以下短文只是 DTO 示例，640/80 多块执行证据见验收记录）：

```json
{
  "title": "品牌声誉复盘",
  "uri": "powerx://solution-proposal/articles/aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
  "content": "品牌舆情发酵时，先核实事实，再公开回应并补救客户损失。",
  "content_type": "text/plain",
  "checksum": "04f155d3c62b4f5f806971265036321d0e2a97628d52b28466a3e40fc5574ebd",
  "version": "v1",
  "idempotency_key": "article-v1-publish-001",
  "ingestion": {
    "schema": "powerx.knowledge.document-ingestion/v1",
    "chunk_size": 640,
    "chunk_overlap": 80
  },
  "indexing": {
    "mode": "hybrid",
    "embedding_profile": {
      "uuid": "11111111-1111-4111-8111-111111111111",
      "version": 1
    },
    "artifact_roles": [
      "source_chunk",
      "knowledge_profile"
    ]
  },
  "external_ref": {
    "document_uuid": "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
  },
  "artifacts": [
    {
      "uuid": "cccccccc-cccc-4ccc-8ccc-cccccccccccc",
      "role": "source_chunk",
      "text": "品牌舆情发酵时，先核实事实，再公开回应并补救客户损失。",
      "checksum": "04f155d3c62b4f5f806971265036321d0e2a97628d52b28466a3e40fc5574ebd",
      "version": "v1",
      "source_ref": {
        "kind": "cleaned_document",
        "source_uuid": "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
        "version": "v1",
        "checksum": "04f155d3c62b4f5f806971265036321d0e2a97628d52b28466a3e40fc5574ebd",
        "position_unit": "unicode_codepoint",
        "char_start": 0,
        "char_end": 27
      },
      "category_codes": [
        "public_relations"
      ],
      "tag_uuids": []
    },
    {
      "uuid": "dddddddd-dddd-4ddd-8ddd-dddddddddddd",
      "role": "knowledge_profile",
      "text": "适用场景：消费者质疑品牌诚信。方法：核实事实、承担责任、持续沟通。",
      "checksum": "42a4aa9a8554cd1c2123b0cc5be1c0d2dfdba927c859a3ea8cda01245c2ac368",
      "version": "v1",
      "source_ref": {
        "kind": "knowledge_profile",
        "source_uuid": "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
        "version": "v1",
        "checksum": "42a4aa9a8554cd1c2123b0cc5be1c0d2dfdba927c859a3ea8cda01245c2ac368",
        "position_unit": "unicode_codepoint",
        "char_start": 0,
        "char_end": 33
      },
      "category_codes": [
        "public_relations"
      ],
      "tag_uuids": [],
      "knowledge_profile_uuid": "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
    }
  ]
}
```

checksum 是完整 UTF-8 文本 SHA256 的 64 位小写 hex，**不加 `sha256:` 前缀**。artifact 版本和 source_ref 版本必须一致；不同 artifact 可以有各自的独立来源版本，Core 冻结本次明确提交的 bundle。业务是否抽取自当前原文由插件证据合同负责。

任务持续使用现有持久化 IndexJob / high-first Worker：queued → running → succeeded/failed。成功意味着完整 artifact 集已实际 Embedding、非零有限向量、维度一致、pgvector 写入数量回读一致、分块与来源保存完成、版本事务发布成功。待发布向量不会进入查询；失败尽力清理 staging，存储不可用时残留也因没有成功的 active bundle 而不可见。

任务回读新增 `artifact_count`、`vector_count`、`bundle_generation`；`effective_config.indexing` 包含完整冻结模型信息。重建返回逐文档 `document_configs[].effective_config.indexing`；不能把多文档配置压成单个配置。计数表示成功发布的产物，失败为零，`processed_documents` 表示实际处理进度。入库请求参数在冻结配置中原样保留，`requested_config` 继续表示旧 ingestion 输入。

重建：`reuse_snapshot` 不接受新 ingestion/indexing；`apply_config` 接受显式新配置。已有语义 artifact 的文档在 apply_config 时必须提供 indexing，禁止意外退回 lexical。切块策略改变不改变稳定 Core document UUID。

## 5. 严格检索

- query 是完整需求文本，直接生成真实向量；不抽关键词，不让调用方传 query embedding/model。Ollama 强制 `truncate=false`，超出模型上下文则失败，严格合同不回退旧 `/api/embeddings`。协议依据：[Ollama 官方 OpenAPI](https://github.com/ollama/ollama/blob/main/docs/openapi.yaml)。
- 四类过滤由 typed 字段控制，并在排序/top_k 前执行；同字段列表为 OR，各字段之间为 AND；tag/category 匹配 any。document_uuids 是 **Core** 文档 UUID。
- semantic：cosine_distance 升序召回，主分数 `cosine_similarity=1-distance`，越大越好；原始 distance 及其方向单独返回。
- hybrid：同时实际执行 vector 和 PostgreSQL `simple` 全文 rank，使用 RRF `sum(1/(60+rank))`；主类型 `rrf`，字面原分类型 `postgres_text_rank`。没有 BM25/rerank 宣称；中文同义召回由向量完成。字面没有匹配是有效空通道，查询执行故障是明确失败。
- hybrid 查询只使用以 hybrid 发布的 artifact；semantic 查询也可使用 hybrid artifact。能力响应的 `indexed_query_modes` 和 `index_counts` 表示实际已发布数据，不能把配置 ready 当成已有索引。
- 返回多个相关 chunk，不按文档去重；包含 artifact/profile/case/Core UUID、external_ref、source_ref、document_version、bundle_generation、逐源分数和 retrieval_sources。hydrate 从成功任务不可变 source 再验证 checksum 和文本偏移，失败返回错误。
- `required_generations` 按空间指定期望 corpus_generation；不一致 409。查询前后及最终多空间返回前再次检查代次和空间资格，避免并发撤销/发布混版。
- 删除/visibility false 在提交事务中立即失效并更新 corpus_generation；不能因失败保护再次显露撤销文档。此服务不使用候选结果缓存；Framework/插件缓存必须包含返回的版本集合并响应撤销。
- 多空间分别生成对应模型向量和返回 generation；检索分数用于候选排序，不是跨模型校准概率或客户适配分。

限制：query ≤8192 codepoints、top_k 1..100、空间 1..16、每类过滤 ≤100、artifact 1..64（文本合计 ≤8 MiB），每文档 ≤4096 chunks，空间任务 ≤1000 文档/65536 chunks。HTTP 文档请求整体限制为 8 MiB +64 KiB，重复原文和 artifact 文本也计入；查询请求 128 KiB。Worker 当前每实例一个槽、执行 deadline 2 分钟、lease 5 分钟；失败明确终态后显式重试。

## 6. 错误与验收

HTTP 400 非法/未知配置；401 无有效凭据；403 缺 grant/RBAC/撤销；404 跨租户或不存在对象；409 代次/幂等/模型绑定/完整空间切换冲突；422 未配置、Profile 失效、不可准确映射来源；501 不支持驱动/数据库布局；503 必需模型、向量、字面通道或 hydration 故障。任务失败在 `error_code` 返回稳定 reason；REST/typed invocation 保留 HTTP 和 reason_code/request_id，不统一改成 502。

真实验收采用隔离 Core 端口 18077/18078，连接实际开发 DB、Ollama、Redis、pgvector；有明确前缀的临时空间、文档、限权 STS/API Key、用户夹具验收后清理。Admin JWT 使用隔离服务的私有测试签名配置，不证明生产账号登录。8077 的用户进程未由本次任务重启。

执行方式（私有配置路径不要提交凭据）：

```sh
cd backend
POWERX_KNOWLEDGE_CONFIG=/path/to/private/config.yaml \
POWERX_KNOWLEDGE_HTTP=http://127.0.0.1:18077 \
POWERX_KNOWLEDGE_GRPC=127.0.0.1:18078 \
POWERX_KNOWLEDGE_EVIDENCE=/absolute/path/semantic-acceptance.json \
go test ./tests/knowledge_space/hostsnapshot -run TestLiveSemanticIndexAndRetrieval -count=1 -v
```

证据包含真正的模型探针、画像/原文向量数量、640/80 的实际分块、同义自然语言排序、前置 top_k 过滤、精确文本位置、多块结果、单篇/空间重建、策略变化、模型待发布与原子切换、故障保留旧版、非法字段、跨租户、只读服务权限、即时 grant 撤销、Admin/RBAC 与 STS 隔离，所有调用保留 HTTP 状态与 Trace。编译/单元测试和真实 API 验收分别列出，不代表 Framework/插件浏览器验收完成。

### 最终验证结果

2026-10-08：最终真实验收通过，共75条记录。初始640/80 bundle为2 artifacts、3 chunks、3 verified vectors；bge-m3→bge-large的完整空间发布为11 vectors。关闭截断后，640分块在较短模型上下文中明确失败；128/16重建成功，并保持切换前旧 generation 可查询。

迁移、capability-check、capability-seed、开发Key grant-status、受影响Go回归、go vet和app构建均通过。当前正式capability总数1022，覆盖936 REST routes；capability-check候选978、引用60、明确ignore6。隔离服务在验证后停止；8077需由使用方重启加载新代码。
