# Framework Go / .NET 语义合同对齐清单

日期：2026-10-08。Core 交付依据：[正式合同](knowledge-semantic-host.md)、[OpenAPI](../../specs/011-knowledge-space/contracts/semantic-host.openapi.yaml)、[真实验收记录](knowledge-semantic-acceptance-20261008.json)。本清单未修改 Framework/插件仓库；消费端完成后仍需自己的运行与浏览器验收。

## 1. 实施顺序

| 顺序 | Framework 修改 | 完成标准 |
| --- | --- | --- |
| F01 | 新增强类型 semantic DTO 与版本枚举 | Go/.NET JSON 与 Core OpenAPI 一致，保留 optional/null/false/0/空数组及顺序 |
| F02 | 扩展 Provider 能力描述/接口与 Runtime Assembly | Local/Delegated 在装配时固定；不支持 semantic 的 Local adapter 返回不支持，不回退 lexical |
| F03 | PowerX Host 的配置/能力/入库/visibility transport | 真实 capability 授权，透传 indexing/artifacts/external_ref/idempotency_key |
| F04 | 严格 Query transport 与结果映射 | 完整 query text、过滤、版本要求、多块/分数/来源逐项保留 |
| F05 | 单篇/空间重建和 Job/Chunks 映射 | DocumentID 非空只调用单篇；冻结 indexing + ingestion，任务终态来自 Core |
| F06 | 错误、Trace、缓存及撤销 | HTTP/reason/Trace 不丢失，403/409/422/501/503 不触发降级或扩大范围 |
| F07 | SDK、适配器和真实 Host 回归 | 完成下文矩阵，提供 request/response/Trace 与模型/向量证据 |

## 2. 代码落点

Framework 根目录：`Core/Plugins/PowerXPlugin/framework`。

| 层 | Go | .NET |
| --- | --- | --- |
| 公共类型/接口 | `backend/go/runtime/knowledge`：types、host_ingestion、provisioning、provider 与 source_policy | `backend/dotnet/src/PowerXPlugin.Framework/Runtime/Knowledge/KnowledgeRuntime.cs`、`KnowledgeDocumentSnapshot.cs`、`KnowledgeProvisioning.cs` |
| Host transport | `backend/go/runtime/powerx/knowledge/client.go` | 当前 Knowledge Host adapter / runtime 实现（以 KnowledgeRuntime 中的装配关系定位） |
| Local/Delegated | `runtime/knowledge/runtime.go`、`local_provider.go`、`delegated_provider.go` | Knowledge Runtime / LocalKnowledgeAdapter / Delegated adapter |
| 测试 | `runtime/powerx/knowledge/*_test.go`、`runtime/knowledge/*_test.go` | `tests/PowerXPlugin.Framework.Tests/Runtime/Knowledge` |

本次核对：Go Host `SearchKnowledge` 仍调用 `/tenant/knowledge/search` 并拒绝 filters/tags/min_score；该路径保持 lexical。`HostIngestion` 已有正式 Core 映射，不能把旧 Local `Ingestion` 混入该字段。新 semantic 查询和 indexing 是新增合同，不能只删除验证或改一个路径。

## 3. 强类型对象

建议对象名称可遵循 Framework 现有风格，**JSON 名称和语义按 Core 固定**：

| 对象 | 必须保留的字段 |
| --- | --- |
| SemanticIndexingSettings | mode、embedding_profile.uuid/version、artifact_roles |
| SemanticIndexSnapshot | 上述输入 + env、configuration_generation、model_key、model_revision、config_checksum、dimensions、vector_index_key |
| SemanticArtifact | uuid、role、text、checksum、version、source_ref；nullable knowledge_profile_uuid/case_uuid；category_codes/tag_uuids |
| SemanticExternalRef | document_uuid，区分插件 UUID 与 Core DocumentID |
| SemanticSourceRef | kind、source_uuid、version、checksum、position_unit、char_start、char_end |
| SemanticQuery | schema、完整 query、space_uuids、mode、top_k、typed filters、required_generations |
| SemanticFilters | document_uuids、category_codes、tag_uuids、artifact_roles；没有 SQL/map 任意键 |
| SemanticResult | schema、query_uuid、requested_mode、effective_mode、generations、items、trace_id |
| SemanticMatch | space/document/chunk/artifact UUID、artifact_role、nullable profile/case UUID、title、text、score/type、scores、retrieval_sources、external_ref、source_ref、document_version、bundle_generation |
| SemanticScore | source、score、score_type、direction；不能用一个 Score 覆盖 distance/similarity/RRF |
| SemanticGeneration | env、space_uuid、configuration_generation、corpus_generation、embedding_profile、model_key/revision、dimensions、distance_metric |
| SemanticCapabilities | configured/ready/indexed、query_modes/index_modes/indexed_query_modes、index_counts、roles/filters/limits、generation、nullable pending_generation、reason_code |
| Visibility | 显式 queryable bool、optional expected_epoch；回读 visibility_epoch |
| IndexJob 增量 | artifact_count、vector_count、bundle_generation、effective_config.indexing、多文档 document_configs[].effective_config.indexing |

version/source checksum 属于不可变来源快照；source version 是字符串，不强制转整数。checksum 为无前缀的小写 SHA256。数组顺序不得被 SDK 自动重排，explicit false/0 不能当 absent。

Go 按 codepoint 使用 `[]rune` 映射位置；.NET 字符串索引为 UTF-16，必须处理 surrogate pair，不能直接用 char_start/char_end 做 Substring，也不能把 grapheme 数当 codepoint 数。加入 emoji/中英混合验收。

## 4. Provider 操作及调用映射

新增或扩展清晰的 typed 方法：

- `GetSemanticCapabilities(space)`。
- `ConfigureSemanticIndex(space, embeddingProfileKey, expectedConfigurationGeneration)`。
- `QuerySemanticKnowledge(SemanticQuery)` / 对应 async 方法。
- `SetDocumentVisibility(document, queryable, expectedEpoch)`。
- 原 UpsertDocument 增加 semantic fields；Reindex 增加 indexing；Job/Chunks 返回真实 Core 状态。

| 调用 | capability | core endpoint / operation |
| --- | --- | --- |
| Capabilities / Query | `com.corex.knowledge.retrieval.read` | `core://knowledge/retrieval` / capabilities、query |
| 配置、提交、资格、重建、任务/分块 | `com.corex.knowledge.document.manage` | `core://knowledge/documents` / configure_semantic_index、submit_document、set_document_visibility、rebuild_document、rebuild_space、get_index_job、get_index_job_chunks |

Delegated 调用 `/api/v1/tenant/invocations`，显式 `preferred_protocol=core_internal`，body 为固定 operation DTO；配置方法使用 `{operation,space_uuid,configuration}`，资格方法使用 `{operation,space_uuid,document_uuid,visibility}`。Query 是 `{operation:"query",query:SemanticQuery}`。解包现有 Registry 的 `data.result`/`data.payload` 后，再解包业务 `result`；不要丢掉两层 Trace。

Local Host simulation 允许按正式表调用租户 REST，必须走有效 API Key/live grant；不能请求 Admin 全权权限。原本插件本地持久化 Local adapter 可以继续 lexical，但不能自称支持 semantic，更不能在 Delegated 失败时被自动装配成 fallback。

## 5. 模型/任务生命周期

1. 空间创建/模型激活后读取 semantic capabilities；`ready` 和 `indexed` 分别显示。
2. 初次 Configure 返回的实际 Profile UUID/version 用于 indexing，不复用 Ingestion/RAG Profile UUID。
3. 有 `pending_generation` 时表示新模型待发布；当前查询使用 active generation，重建用 pending 的 Profile ref。
4. 模型更换需要用户明确执行空间 apply_config 重建。单篇不能偷偷升级整个空间模型；`KNOWLEDGE_MODEL_SPACE_REBUILD_REQUIRED` 应明确显示。
5. retry 保留原请求配置；同次网络重发保留 idempotency key，失败后的新执行用新 key。同键冲突不自动改 key 掩盖错误。
6. queued/running 只轮询；succeeded 才发布结果。失败读取真实 error_code、Trace 和旧 active 版本，不用本地 CompletedIndexJob 伪造完成。
7. apply_config 对语义文档必须传 indexing；reuse_snapshot 不传新 ingestion/indexing。单篇与空间 endpoint 依 DocumentID 明确选择。
8. Profile/模型/配置修订不能修改旧任务快照。任务多文档配置不能仅读取第一项。

## 6. 分数、错误和缓存

- 保留 `cosine_distance/lower_better`、`cosine_similarity/higher_better`、`postgres_text_rank/higher_better`、`rrf` 的独立类型，不映射成客户适配分/百分比。
- 不对 semantic 结果使用 legacy MinScore 默认值、不丢同文档的其他分块、不自行造来源 URI/页码/置信度。
- 错误对象至少包含 HTTP status、reason_code/error_code、request_id/trace_id、原始受控 details；所有非 2xx 和 failed Job 都可明确定位。
- 401 可按既有凭据刷新策略有限重试；403 权限撤销、409 版本/幂等冲突、422 Profile/配置错误、501 不支持、503 必需通道故障不能变成成功空结果或 lexical 搜索。
- 默认不缓存候选。若启用缓存，key 包含 tenant/授权主体、query、全部 filters、mode、模型配置与逐空间 corpus generation；撤销/删除必须失效。UI 不靠缓存维持旧资格。

## 7. 验收矩阵与交付材料

- [ ] Go/.NET request/response JSON 对齐，包括 null、false、0、空数组、顺序、opaque 版本和 Unicode 位置。
- [ ] Local lexical adapter 报不支持 semantic；Delegated 故障没有 Local fallback。
- [ ] 真正 API Key 和 STS 查询/入库；grant-status=granted；只读 key 写入 403；撤销后立即 403；STS Admin 403。
- [ ] 画像+原文真实向量入库；640/80 snapshot/chunks/vector_count 回读一致。
- [ ] 完整自然语言同义召回；top_k 前 document/category/tag/role 过滤；同文档多分块；源文字区间准确。
- [ ] 保留原始距离与相似度方向、RRF/字面 score 类型；不计算业务 fit score。
- [ ] 单篇、空间、跨租户、非法配置、失效 Profile、同键重发/冲突、执行中冲突、失败重试、策略变化。
- [ ] 新模型 pending → 单篇不得整体切换 → 空间原子发布；失败旧配置/索引继续有效。
- [ ] required_generations 冲突 409；资格/删除不可见；来源损坏/模型/向量通道故障显式失败。
- [ ] 插件再完成浏览器多文件提交 → Job 终态 → 快照/分块/来源 → 真实检索与逐维度评分验收。

消费端交付修改文件、Go/.NET 测试结果、真实调用 HTTP+Trace、模型/维度/vector_count、缓存/撤销行为和待验收范围。Core 的隔离验收不替代 Framework 或插件验收。
