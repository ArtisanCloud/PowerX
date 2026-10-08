# Knowledge Lab：Core 策略目录与空间创建合同

适用：Framework Local+Host proxy（开发 API Key）及 Delegated（安装插件 STS）。Framework 需实现 typed catalog/create 并声明支持操作，插件再接表单。本合同不表示浏览器或安装插件验收已经完成。

## 路由及授权

| 操作 | Capability | 固定 binding | Tenant Host REST |
|---|---|---|---|
| 目录 | `com.corex.knowledge.catalog.read` | `INVOKE core://knowledge/catalog` | `GET /api/v1/tenant/knowledge/catalog` |
| 创建 | `com.corex.knowledge.space.create` | `INVOKE core://knowledge/spaces` | `POST /api/v1/tenant/knowledge/spaces` |

Admin 目录：`GET /api/v1/admin/knowledge-spaces/catalog`。Admin 创建沿用 `POST /api/v1/admin/knowledge-spaces` 和既有 camelCase DTO；Framework 使用 service DTO。

显式 Key 权限：`_scope.knowledge.catalog.read` + `read api/catalog`；`_scope.knowledge.space.create` + `create api/space`。同时验证正式发布、租户注册、有效 Key 和 live grant。STS 验证认证上下文和实例 `auth.credentials.allowed_capabilities` 精确 grant。无授权 403；无认证由认证层返回 401。租户和 actor 只取可信凭证。能力发布版本变化后，已有租户版本锁可能需管理员确认升级。

网关使用既有 `/api/v1/tenant/invocations` 包装，以下是 `payload`：

```json
{"method":"INVOKE","endpoint":"core://knowledge/catalog","body":{"operation":"catalog"}}
```

```json
{
  "method":"INVOKE",
  "endpoint":"core://knowledge/spaces",
  "body":{
    "operation":"create",
    "name":"产品知识库",
    "department_uuid":"<当前租户有效部门 UUID>",
    "strategy_key":"A_simple",
    "scene_key":"support_faq",
    "policy_template_uuid":"<目录返回 UUID>"
  }
}
```

REST 创建 body 同上但不带 operation。禁止任意 method/endpoint/query/header/raw proxy，以及 tenant_uuid/actor 字段。

## 目录与创建字段

目录返回 REST `data.catalog` / 网关 `payload.catalog`：

- version、source=powerx_core；scenes 含 key/label/description/default_bundle/allowed_bundles。
- strategy_packages 含 key/label/summary/recommended_profile_key/recommended_scenes/dependencies（index/runtime/assets）。
- profiles 返回当前租户对应 key 最新 published 的 ingestion/index/rag，含 uuid/key/version；缺失则省略。
- available 表示可创建，unavailable_reasons 明确缺少 Profile、embedding、模板或 runtime dependency。未知 runtime dependency 不假定支持。
- activation_dependencies 单列 index/assets；这些依赖可在后续导入和激活生成。可创建不代表索引已就绪。
- policy_templates 是 Core 全局策略模板 uuid/name/version；default_policy_template_uuid 仅在 default/v1 实际存在时返回。
- quota_defaults 为 cpu_cores=4、storage_gb=200、ingestion_concurrency=2；quota_minimums 为 1、50、1。来源为 Core 创建合同默认值和既有最小约束，不是计费额度。quota_override_allowed=true 表示本服务 grant 可设置合法配额。

目录只读，不创建模板/Profile、不补历史 UUID、不用前端默认目录掩盖失败。

创建必填 name（非空，最多 128 bytes）、department_uuid、strategy_key。可选：

- scene_key 省略取策略第一个推荐场景，显式值必须属于 recommended_scenes。
- policy_template_uuid 省略选择实际 default/v1；无默认时必须显式选择。
- ingestion_profile_uuid/index_profile_uuid/rag_profile_uuid 省略取目录映射；显式值必须匹配最新 published 映射，否则刷新目录。
- quotas 整体省略取默认，传入时三项均须满足最小值。

部门必须属于凭证租户且 active；跨租户/失效部门 403。事务内再次验证 Department/Profile。内部 code 保留兼容快照。策略 key、场景和 Profile key 不可混用。

沿用现有 CreateSpace 事务创建空间、IAM 任务、审计，任一步失败全部回滚；重复同租户名称 409。返回 REST `data.item` / 网关 `payload.item` 包含 space_uuid/name/status/department_uuid/strategy_key/scene_key/policy_template_uuid/profiles/quotas。status 初始为 pending_iam；后续沿用 IAM 同步及索引激活，再用现有目录复读。

Profile UUID 持久化为创建时快照；运行时仍遵循既有 Profile key 解析，本轮不改变为全生命周期版本锁定。

## 错误

400：非法字段/UUID/策略/场景/模板/Profile/配额或自由代理 payload。
403：无 grant、跨租户或不可用部门。
409 KNOWLEDGE_SPACE_CONFLICT：重复名称。
412 KNOWLEDGE_STRATEGY_UNAVAILABLE：目录列出的创建依赖未满足。
503：目录、存储或 UUID 迁移依赖不具备。

Framework 保留原状态、machine code 和 Core Trace ID，不能转换为 unsupported capability。

## 部署及验收

**需要标准 Core migrate**：新增策略模板公开 UUID、空间 Department/Profile UUID nullable 快照。集中迁移幂等补齐历史模板 UUID，保留 numeric 主键和历史引用。迁移前按现有流程备份。回滚应用可保留新增列，不重新生成 UUID。

1. Core migrate → make capability-check → make capability-seed。
2. 开发 Key 两项精确 grant；开发 seed 已包含 scope，但 capability-seed 本身不授予 Key。
3. STS 安装实例声明并授权两项能力，保留现有空间目录 grant。
4. 重启 Core，真实 grant-status 检查，需要时管理员确认能力版本升级。
5. API Key/STS 分别 catalog → 真实部门和 available 策略 → create → 目录复读，记录 Trace ID。
6. 验证无 grant/跨租户 403、非法 Profile 400、重复名称 409；失败无空间/IAM/audit 残留。
7. Framework OperationCatalog + typed create 接入后，插件解除禁用并完成浏览器验收。

代码：host_provisioning.go、provisioning_capability_invoker.go、Tenant knowledge_space/routes.go、platform_capabilities/knowledge.yaml。OpenAPI 见 specs/011-knowledge-space/contracts/host-provisioning.openapi.yaml。

## 本轮验证状态（2026-10-04）

相关 8 个 Go 包测试通过，覆盖两种可信凭证上下文的 typed catalog/create/复读、精确 grant、跨租户拒绝、非法字段/自由 endpoint 拒绝、事务审计故障回滚、迁移幂等和已注册 HTTP 路由。make capability-check 及 capability-seed 成功；seed 只发布能力和权限元数据，不签发或轮换运行凭证。

真实开发 Host Key grant-status HTTP 200，两项当前均为 not_granted，request_id：6f45d092-2d9b-4784-8076-209f883afdf9。尚未执行新增 schema 的全库 migrate、重启用户 Core 或授予现有 Key/安装实例的新 grant。真实 API Key/STS 创建及浏览器验收仍待完成，测试中的可信上下文不能视为真实 STS Exchange 验收。

### 实际迁移及目录复验（2026-10-04）

按用户授权，使用 8077 进程的 POWERX_CONFIG 执行标准 `cmd/database migrate`，退出码 0。数据库为 localhost:5432/powerx；策略模板 1 条，空或零值 UUID 为 0。
配置仍含公开默认 JWT 值，迁移进程单独使用临时随机环境值通过校验；没有修改配置文件或运行服务的签名密钥。
真实开发 API Key 调用 `GET /api/v1/tenant/knowledge/catalog` 返回 HTTP 200，source=powerx_core，策略包 18 项、模板 1 项。X-Trace-ID：cf7f1e4c-623e-470f-9e36-7ade6978596c；request_id：b5c72d0c-9855-411b-acf4-9b42b1c022fb。
本次没有重复授权或重启 Core；上述记录取代前文“尚未 migrate”的状态。STS 创建和插件浏览器创建验收仍未在本次执行。

## 文档入库配置快照 v1（2026-10-07）

### 支持范围与授权

提交：`POST /api/v1/tenant/knowledge/spaces/{space_uuid}/documents`；任务回读：`GET /api/v1/tenant/knowledge/index-jobs/{job_uuid}`；不可变分块回读：`GET /api/v1/tenant/knowledge/index-jobs/{job_uuid}/chunks`。三者复用 `com.corex.knowledge.document.manage`、`_scope.knowledge.document.manage`、action=`manage`、resource_type=`api`、resource_pattern=`document`。已具有该 grant 的 API Key 不需要重复授权。STS 实例凭据的 `auth.credentials.allowed_capabilities` 必须包含该能力，并满足已发布能力、租户注册及有效凭据校验。

同一正式能力还声明：

- Admin：用户 JWT + RBAC `corex.knowledge.document:manage`，提交 `/api/v1/admin/knowledge-spaces/{spaceId}/documents`，回读 `/api/v1/admin/knowledge-spaces/index-jobs/{jobId}` 及 `/chunks`。STS/API Key 不能代替 Admin 用户权限。
- 服务 binding：`INVOKE core://knowledge/documents`，operation=`submit_document` / `get_job` / `get_job_chunks`。禁止任意 HTTP method、endpoint、headers、query、tenant/actor 覆盖和 raw proxy。服务调用使用可信 request context，不注入代理 HTTP headers。

目录的 `document_ingestion` 正式公布 schema、支持格式/模式、默认值和限制。策略目录增加稳定 `uuid`、`version`，文档可使用策略 UUID+版本或 key+版本，必须匹配空间已绑定策略；策略覆盖不支持。Profile 始终使用当前租户已发布的精确 UUID/版本；不会把 key 或 latest 当成对象引用。

本版本执行 **TXT/Markdown → 实际分块 → 持久化 lexical chunk index**，`index_mode=lexical_chunks`。它不是向量 embedding 入库合同；原向量入库入口继续保留。RAG/Index Profile 引用和配置副本随任务冻结，任务不能覆盖其空间绑定。独立 processor/masking Profile、语义模型分段、PDF/OCR 分页、RAG 执行计划覆盖目前不由此端点支持，明确返回 422；不能把这些设置丢弃后返回成功。

### 正式提交结构

基础字段仍为 `title/uri/content/content_type/checksum/version/tags`。`checksum` 必须等于 UTF-8 content 的 SHA-256。可选 `ingestion` 为嵌套 snake_case 对象：

```json
{
  "title": "退款规则",
  "uri": "powerx://documents/refund-policy",
  "content": "<完整原文>",
  "content_type": "text/markdown",
  "checksum": "<UTF-8 原文 SHA-256>",
  "version": "source-v1",
  "tags": ["refund"],
  "ingestion": {
    "schema": "powerx.knowledge.document-ingestion/v1",
    "ingestion_profile": {"uuid": "<当前租户 published Profile UUID>", "version": 7},
    "priority": "high",
    "segment_mode": "unit",
    "segment_size_policy": "cap",
    "chunk_size": 640,
    "chunk_overlap": 80,
    "segment_order": ["page", "segment", "separator", "size"],
    "separators": [],
    "page_priority": false,
    "anchor_heading_path": false,
    "anchor_clause_id": false,
    "anchor_row_number": false,
    "anchor_speaker": false,
    "anchor_sentence_index": false
  }
}
```

服务 binding 的 body 为 `{"operation":"submit_document","space_uuid":"<UUID>","document":<上述文档对象>}`；任务 body 为 `{"operation":"get_job","job_uuid":"<UUID>"}` 或 `get_job_chunks`。外层 tenant invocation 按已有协议设置 `capability_id` 和 `preferred_protocol=core_internal`。

| 字段 | 默认及执行约束 |
|---|---|
| ingestion | 省略/null 保留基础文本入口，冻结空间 Profile 和默认值；显式对象必须声明 schema |
| ingestion_profile | 默认空间绑定 UUID；显式引用必须是本租户已发布 UUID+精确整数版本 |
| processor_profile / masking_profile | 仅省略/null；非 null 返回 `KNOWLEDGE_UNSUPPORTED_INGESTION_SETTING` |
| priority | normal；支持 normal/high。持久化队列领取时 high 优先，同级按提交时间，不抢占已经运行的任务 |
| segment_mode | unit；支持 unit/heading/clause/table_row/code_block/conversation。heading/table_row/code_block 要求 Markdown；semantic 明确拒绝 |
| segment_size_policy | cap；支持 cap/target，target 要求 chunk_size 大于零 |
| chunk_size | 1024，单位 Unicode rune，范围 0..65536。零关闭长度窗口，仍按分段模式处理，要求 overlap=0、policy=cap |
| chunk_overlap | 0；正长度时必须小于 chunk_size，负数/超限拒绝，不能自动改为其他值 |
| segment_order | page/segment/separator/size，各出现一次，完整数组的原顺序实际执行 |
| separators | 空数组；最多 16 个唯一非空字面分隔符，每项最多 32 rune。JSON `\n` 解码为换行；二次转义的反斜杠 n/t 被拒绝。换行/空白分隔符实际参与处理 |
| page_priority | false；TXT/Markdown 中 true 明确拒绝，不把源文本假装成 PDF 页面 |
| anchors | 默认 false；heading_path 对应 heading，clause_id/sentence_index 对应 clause，row_number 对应 table_row，speaker 对应 conversation，不兼容组合拒绝 |

生效顺序为：显式请求 → 已冻结 ingestion Profile 的 `config.chunking` → Core 默认。false、0、空数组和数组顺序均保留。文档限制 8 MiB；每任务正文分块上限 4096，超出窗口/边界预算提前拒绝。独立高级处理 Profile 未支持时，不通过基础文本路径暗中降级。

### 不可变性、任务状态与产物

受理事务同时保存文档及任务：请求配置、生效配置、三个空间 Profile 的 UUID/版本和配置副本、策略标识/版本、文档版本/原文/checksum、trace_id。生效快照使用规范化 JSON SHA-256，兼容 PostgreSQL JSONB 的键序和空白变化。Profile 配置完整副本仅保留在任务内部，公开回读提供 UUID/key/version/config_checksum。

Worker 只读取任务冻结副本，不回读“最新 Profile”或最新文档内容。正文 chunk 属于不可变 job_uuid，后续文档更新不会覆盖旧任务产物。每条 chunk 返回 content/checksum/ordinal 和来源、分段、锚点、job_uuid、snapshot_checksum 等 metadata；任务成功必须发生在真实 chunk 持久化之后。检索只投影当前文档版本已成功任务的实际 chunks。

受理返回 202 的 queued job；详情返回 queued/running/succeeded/failed、priority、trace_id、started_at/completed_at、chunk_count、snapshot_checksum、requested_config 和 effective_config。任务中断/租约失效明确失败为 `KNOWLEDGE_WORKER_INTERRUPTED`，无 snapshot 的旧未完成任务不会被伪造为成功。非抢占单实例执行槽及跨实例行锁/claim token 防止重复领取；原始任务配置不随状态更新重写。

相同 URI、原文 checksum 和生效快照已受理时返回 409；配置或文档版本改变可以创建新任务，失败任务允许重新提交。删除不销毁历史任务 chunks；2026-10-07 后续重建合同已改为冻结原文并实际重新分块，详见下节；旧复制分块的历史任务保留，但不能用它推断新配置已执行。

详情示意（真实例证见验收记录）：

```json
{
  "job_uuid": "<UUID>",
  "status": "succeeded",
  "operation": "upsert",
  "priority": "high",
  "trace_id": "<Core Trace UUID>",
  "chunk_count": 4,
  "snapshot_checksum": "<canonical JSON SHA-256>",
  "effective_config": {
    "schema": "powerx.knowledge.document-ingestion/v1",
    "index_mode": "lexical_chunks",
    "length_unit": "unicode_rune",
    "chunk_size": 640,
    "chunk_overlap": 80,
    "ingestion_profile": {"uuid": "<UUID>", "key": "<key>", "version": 7, "config_checksum": "<SHA-256>"}
  }
}
```

### 错误与验收

- 400 `KNOWLEDGE_INVALID_ARGUMENT`：错误长度、重叠、顺序、非法 UUID、checksum 不匹配、未知字段、缺 schema。
- 401 `KNOWLEDGE_UNAUTHORIZED` 或认证层错误：没有有效凭据。
- 403 `KNOWLEDGE_FORBIDDEN` 或认证层拒绝：缺对应 grant、未注册能力或服务凭据无授权。
- 404 `KNOWLEDGE_SPACE_NOT_FOUND` / `KNOWLEDGE_INDEX_JOB_NOT_FOUND`：对象不属于当前凭证租户，避免泄露跨租户对象。
- 422 `KNOWLEDGE_PROFILE_UNAVAILABLE`：跨租户 Profile、版本不匹配、draft/archived Profile。
- 422 `KNOWLEDGE_UNSUPPORTED_INGESTION_SETTING`：不能实际执行的配置。
- 执行终态 failed：`KNOWLEDGE_INDEX_EXECUTION_FAILED`、`KNOWLEDGE_SNAPSHOT_INVALID` 或 `KNOWLEDGE_WORKER_INTERRUPTED`；详情保留 trace_id。

以上提交校验错误发生在写入之前，不遗留文档或任务。迁移集中挂载于 `pkg/corex/db/database/migration.go`，包含 IndexJob 新字段、文档当前任务引用及 immutable Host chunk 表。升级必须先按同一服务配置运行 database migrate，再重启 Core；不需要 Framework/Plugin UI 改动才能验证 Core。

可选真实验收测试（使用私有配置、当前真实开发 API Key，通过真实 gRPC Exchange 取得 STS；只创建并清理带前缀的测试资料/限权凭据）：

```sh
(cd backend && POWERX_KNOWLEDGE_CONFIG=/absolute/private/config.yaml POWERX_KNOWLEDGE_HTTP=http://127.0.0.1:18077 POWERX_KNOWLEDGE_GRPC=127.0.0.1:18078 POWERX_KNOWLEDGE_EVIDENCE=/absolute/private/evidence.json go test ./tests/knowledge_space/hostsnapshot -run '^TestLiveDocumentConfigurationSnapshot$' -count=1 -v)
```

不得将孤立测试 JWT、仅 202 或仅 DTO JSON 当作真实验收。证据须包含实际配置回读、chunk 文本/校验/重叠、Profile 修改后的旧任务回读、API Key grant-status、STS Exchange、非法配置/跨租户/失效版本状态与 Trace。

### 已执行验收记录

2026-10-07：官方集中 migrate 已通过；`make capability-check` 为 declared=1021，`make capability-seed` 同步 1021 个能力、4 个租户注册；现有开发 API Key 的 document.manage `grant-status=granted`。在独立 18077/18078 实例完成真实 API Key 与 STS HTTP 提交/回读及固定 typed Core binding，HTTP/Trace 见 [脱敏证据](evidence/knowledge-document-snapshot-20261007.json)。正文 2000 rune 每份生成四个正文块，每块不超过 640，相邻块重叠 80；Profile 配置修改/归档与空间绑定修改后旧任务快照不变；非法请求未额外留下任务或文档。测试夹具和限权 STS 凭据已清理，证据不含 Key/secret/token。

通过的代码验证包括 Knowledge Space 完整包测试、Host/分块竞态回归、相关 HTTP/能力/API-key 包竞态测试、go vet、后台编译、YAML 解析和 diff 检查。正式 8077 保持用户原进程，生效仍需按原启动方式重启；此次独立服务验收不等同于插件浏览器多文件验收。


## 单篇与空间重新处理合同（2026-10-07）

本节替代此前空间重建复制已有 chunks 的行为。单篇、整个空间现在均从受理时冻结的原文实际执行分段和切块，创建独立任务产物；文档 UUID 不变。

| 范围 | Tenant REST | typed core_internal operation |
|---|---|---|
| 单篇 | `POST /api/v1/tenant/knowledge/spaces/{space_uuid}/documents/{document_uuid}/indexes:rebuild` | `rebuild_document` |
| 整个空间 | `POST /api/v1/tenant/knowledge/spaces/{space_uuid}/indexes:rebuild` | `rebuild_space` |

精确授权沿用 `com.corex.knowledge.document.manage` 及 `_scope.knowledge.document.manage` 的 manage/api/document grant，API Key 和 STS 继续由可信凭证决定租户。单篇同时校验 tenant + space + document，跨租户/不在指定空间返回 404。Admin 单篇入口为 `/api/v1/admin/knowledge-spaces/{spaceId}/documents/{documentId}/indexes:rebuild`，仅用户 JWT/RBAC；服务 binding 固定 `INVOKE core://knowledge/documents`，不能注入任意 endpoint/headers/tenant/actor。

### 请求及冻结语义

沿用上次成功配置：

```json
{"mode":"reuse_snapshot","idempotency_key":"<一次用户操作的唯一键>"}
```

应用显式新配置：

```json
{
  "mode":"apply_config",
  "idempotency_key":"<另一次操作的唯一键>",
  "ingestion":{
    "schema":"powerx.knowledge.document-ingestion/v1",
    "ingestion_profile":{"uuid":"<本租户 published UUID>","version":7},
    "chunk_size":320,
    "chunk_overlap":40
  }
}
```

- 空 body 或省略 mode 默认 `reuse_snapshot`，兼容既有空间重建客户端。
- `reuse_snapshot` 禁止 ingestion；使用目标文档最后成功任务中冻结的配置，Profile 后来修改或归档不改变它。不存在有效成功快照返回 422，并要求使用 apply_config。
- `apply_config` 必须提供 ingestion；重新校验 Profile 发布状态、精确版本、租户、支持矩阵和真实执行预算，不隐式切换 latest。
- 两种模式的原文均为**受理时当前文档版本**，保存 title/URI/content/version/checksum。它不是回放历史原文接口。
- 空间任务按文档冻结各自的来源、配置及基准索引引用，不能仅保存一组最新空间参数。最多 1000 篇、原文总量 32 MiB、产物总量 65536 正文块；单篇仍受原有 4096 分块约束。
- 幂等键支持 body 或 `Idempotency-Key` header，同时提供时必须一致。键最长 128 字符；相同租户/空间下，同键同请求意图返回原任务，即使已经终态；同键不同目标/模式/配置返回 409。未提供由服务生成新键，客户端应按用户一次操作生成稳定键。
- 同篇文档已有 queued/running upsert/rebuild/delete 时，新操作返回 409。活动空间重建与该空间的文档写入互斥，不能绕过单篇范围限制。
- 失败后用**新幂等键**重试，创建新任务；旧失败记录继续保留。不能把重新发同一键理解为重新执行。

服务 binding 的 body 示例：

```json
{
  "operation":"rebuild_document",
  "space_uuid":"<UUID>",
  "document_uuid":"<UUID>",
  "rebuild":{"mode":"reuse_snapshot","idempotency_key":"<唯一键>"}
}
```

`rebuild_space` 不允许 document_uuid；Framework 必须根据是否有 DocumentID 选择对应操作，不能将单篇降为全空间。

### 成功切换与失败保留

文档最新尝试 `index_job_uuid` 与最后成功的 `active_index_job_uuid` 分离。执行期间，检索仍读旧成功 chunks；只有所有新产物保存成功并确认来源版本、领取 token 和租约仍有效后，才在同一事务中更新 active 指针及任务 succeeded。空间重建全部文档一起原子切换；任一篇失败则整个任务失败，其他文档也不能提前切换。

上次成功 chunks 和快照保留；失败不会把有成功索引的文档标成不可检索。查询标题、URI、tags 也取当前成功产物的冻结来源，避免新原文尚未成功却混入旧索引引用。提交/重建/删除通过空间行锁和活动任务检查串行受理；跨 Core Worker 继续使用 claim token 和租约。

集中 migrate 新增 active 指针、幂等键/请求摘要、rebuild_mode、document_count、processed_documents。历史已成功索引回填 active 指针。任务详情继续返回状态、失败码、trace_id、chunk_count；新增 processed_documents/document_count 表示真实分块进度。单篇提供 effective_config；空间任务提供按文档分组的 document_configs，不能假装所有文章使用同一策略。

### 验收及交付边界

Core 回归覆盖单篇、全空间、UUID 稳定、另一篇不受影响、幂等重复/冲突、queued/running 互斥、跨租户、非法配置、Profile 变更、归档后 reuse、故障后旧索引检索和新任务重试。真实 API Key + gRPC Exchange 签发 STS 的 HTTP 证据见 [重建验收记录](evidence/knowledge-document-rebuild-20261007.json)。故障注入仅修改本测试受理任务的快照，不修改用户任务或全局服务；临时测试资料/凭据结束后清理。

复现：在上一节真实验收命令中增加 `POWERX_KNOWLEDGE_REBUILD_CHECK=1`。本轮只交付 Core；Framework Go/.NET 的快照映射、DocumentID 路由选择及插件展示由消费端接入。生效需迁移后重启正式后台；独立 18077/18078 的验收不是插件浏览器验收。


## dev 历史空间 Profile 绑定修复（2026-10-08）

已修复空间 dev（30a6d654-c4c5-4a33-9806-6d66c6728187），租户 6b5d0240-9920-46da-b707-88200e0f51ea：

| 绑定字段 | 同租户已发布 p1_general v1 UUID |
| --- | --- |
| ingestion_profile_uuid | 4245b851-0710-4566-9f53-ecf3dd7e04cc |
| index_profile_uuid | 81e39770-93dd-4903-918c-9847224d30a4 |
| rag_profile_uuid | aa8c8f9c-6558-435d-a352-4727adb8163f |

受控事务锁定空间和精确目标 Profile，校验租户、类型、key、版本、published 状态及当前策略/场景；不允许拿目录首项代替绑定。只更新三个 UUID 和 updated_at；feature_flags 完整保留，包含 h_fusion、product_specs、p1_general、quota.ingestion:2。空间状态、部门、配额、策略模板、Embedding Profile 和现有向量索引未改动。重复执行无字段变化。可复查脚本见 repairs/dev-profile-bindings-20261008.sql。

在当前8077服务使用既有、已授予 document.manage/search.read 的开发 API Key 验收，无重复授权或服务重启：

| 场景 | 提交 | 任务 | 快照/分块 | 实际分块 Unicode rune 长度 | 检索 |
| --- | --- | --- | --- | --- | --- |
| 插件原配置 unit、1024/0 | 202 | succeeded | 200 | 1024、976 | 200，命中对应文档 |
| unit、640/80 | 202 | succeeded | 200 | 640、640、640、320；相邻重叠80 | 200，命中对应文档 |

两份快照均回读到上述三个精确 UUID、版本1及 h_fusion 策略，源文和分块 checksum 已逐项校验。验收样本标题以 Core dev Profile绑定验收 开头，URI 使用 powerx://core-acceptance/dev-profile-bindings-20261008/ 前缀，保留在 dev 便于回读；没有重试或替换插件的五份原始文件。

默认配置任务：4dc1584f-4fae-4a7c-8e8d-56b6c17d69d4，提交 Trace 5fc457f0-2965-4f7b-8d73-a8c4624cffa5。
640/80任务：5c7e325f-7c9c-45a6-9322-88a8769b96fb，提交 Trace 8230336b-a62e-4c02-b015-0fb87738c2c7。
HTTP 状态、request_id、Trace、原绑定/新绑定和真实响应见 evidence/knowledge-dev-profile-bindings-20261008.json，证据不含凭据。

Core 数据修复和真实 API 验证已完成。消费端接下来使用插件的“重试失败文件”验证原文件队列、任务终态、快照/分块及检索；本节不将 Core API 验收当作插件浏览器验收。本次索引仍为 lexical_chunks，不宣称已完成向量 Embedding 入库。
