# Metadata Governance 机制设计

## 核心对象

## i18n 存储约定

`name_i18n`、`label_i18n`、`description_i18n` 统一使用 JSONB map，不使用自由文本，也不在记录中直接写死单语言文案。

示例：

```json
{
  "zh-CN": "客户等级",
  "en-US": "Customer Level",
  "ja-JP": "顧客ランク",
  "ko-KR": "고객 등급"
}
```

约束：

- `zh-CN` 为首版必填 locale。
- API 返回当前 locale 命中的 `display_name` / `display_description`，同时返回原始 i18n map 供管理端编辑。
- 若请求 locale 未命中，返回明确字段级错误或由调用方显式指定 fallback locale；服务端不做静默语言 fallback。
- `namespace`、`code`、`resource_type` 是机器语义标识，不参与 i18n。

### 数据字典

用途：为业务字段提供稳定枚举。

建议表：

- `metadata_dictionary_namespaces`
  - `uuid`
  - `tenant_uuid`
  - `namespace`
  - `module`
  - `name_i18n`
  - `description_i18n`
  - `status`
  - `created_at`
  - `updated_at`
- `metadata_dictionary_items`
  - `uuid`
  - `tenant_uuid`
  - `namespace_uuid`
  - `code`
  - `label_i18n`
  - `description_i18n`
  - `sort_order`
  - `status`
  - `metadata`
  - `reference_count`
  - `created_at`
  - `updated_at`

唯一约束：

- `tenant_uuid + namespace`
- `tenant_uuid + namespace_uuid + code`

### 分类体系

用途：表达可控层级结构。

建议表：

- `metadata_taxonomies`
  - `uuid`
  - `tenant_uuid`
  - `namespace`
  - `module`
  - `name_i18n`
  - `description_i18n`
  - `max_depth`
  - `status`
  - `created_at`
  - `updated_at`
- `metadata_taxonomy_nodes`
  - `uuid`
  - `tenant_uuid`
  - `taxonomy_uuid`
  - `parent_uuid`
  - `code`
  - `label_i18n`
  - `description_i18n`
  - `path`
  - `depth`
  - `sort_order`
  - `status`
  - `reference_count`
  - `created_at`
  - `updated_at`

唯一约束：

- `tenant_uuid + namespace`
- `tenant_uuid + taxonomy_uuid + code`
- `tenant_uuid + taxonomy_uuid + parent_uuid + label_i18n_digest`

### 标签

用途：给业务实体打标，支持筛选、推荐、统计和 Agent 上下文组织。

建议表：

- `metadata_tags`
  - `uuid`
  - `tenant_uuid`
  - `namespace`
  - `resource_type`
  - `code`
  - `label_i18n`
  - `color`
  - `status`
  - `source`
  - `usage_count`
  - `created_at`
  - `updated_at`
- `metadata_tag_bindings`
  - `tenant_uuid`
  - `tag_uuid`
  - `resource_type`
  - `resource_uuid`
  - `created_by_uuid`
  - `created_at`

唯一约束：

- `tenant_uuid + namespace + resource_type + code`
- `tenant_uuid + tag_uuid + resource_type + resource_uuid`

`metadata_tag_bindings` 是中间表，可以没有 `uuid`，但 `tag_uuid` 和 `resource_uuid` 必须引用业务对象的 `uuid`。

## 资源引用与完整性

标签绑定面向多种业务对象，数据库无法对 `resource_uuid` 建立单一外键。首版采用 `resource_type registry + service 校验`：

- 底座维护 `metadata_resource_types` 注册表。
- 每个可被打标签的业务对象必须注册 `resource_type`、所属模块、显示名称 i18n、可选的引用校验器。
- 创建或替换标签绑定时，service 必须校验 `resource_type` 已注册。
- 若该 `resource_type` 提供对象存在性校验器，写入前必须确认 `resource_uuid` 存在且属于当前租户。
- 若该 `resource_type` 未提供校验器，首版不允许写入绑定，返回明确错误。

建议表：

- `metadata_resource_types`
  - `uuid`
  - `tenant_uuid`
  - `resource_type`
  - `module`
  - `name_i18n`
  - `description_i18n`
  - `status`
  - `created_at`
  - `updated_at`

唯一约束：

- `tenant_uuid + resource_type`

## 引用计数与删除保护

字典项、分类节点、标签都存在“被业务数据引用后不能硬删除”的规则。首版不做跨业务表扫描，采用显式引用登记：

- `metadata_references`
  - `tenant_uuid`
  - `metadata_type`
  - `metadata_uuid`
  - `resource_type`
  - `resource_uuid`
  - `field_name`
  - `created_at`

唯一约束：

- `tenant_uuid + metadata_type + metadata_uuid + resource_type + resource_uuid + field_name`

行为：

- 业务模块写入字典项、分类节点或标签绑定时，同步维护引用登记。
- 删除前只检查 `metadata_references` 或 `metadata_tag_bindings`，不临时扫描业务表。
- 引用存在时硬删除返回冲突错误；调用方必须先迁移引用或停用对象。
- `reference_count` / `usage_count` 可以由查询聚合或异步刷新维护，不能作为唯一删除依据。

## 分类节点路径和移动

`metadata_taxonomy_nodes.path` 使用节点 UUID 路径，不使用 label 或 code 路径。

示例：

```text
/taxonomy_uuid/root_node_uuid/child_node_uuid
```

移动节点规则：

- 移动必须在事务内完成。
- 必须检查目标父节点属于同一 taxonomy。
- 必须检查不能把节点移动到自身或自己的后代下面。
- 必须重新计算当前节点及所有后代的 `path`、`depth`。
- 新深度不能超过 taxonomy 的 `max_depth`。
- 并发移动使用 `updated_at` 或 `version` 做乐观锁；冲突时返回明确错误。

## API 契约

管理端 canonical REST 路径：

- `GET /api/v1/admin/metadata/dictionaries`
- `POST /api/v1/admin/metadata/dictionaries`
- `GET /api/v1/admin/metadata/dictionaries/:namespace_uuid/items`
- `POST /api/v1/admin/metadata/dictionaries/:namespace_uuid/items`
- `PATCH /api/v1/admin/metadata/dictionary-items/:item_uuid`
- `DELETE /api/v1/admin/metadata/dictionary-items/:item_uuid`
- `GET /api/v1/admin/metadata/taxonomies`
- `POST /api/v1/admin/metadata/taxonomies`
- `GET /api/v1/admin/metadata/taxonomies/:taxonomy_uuid/nodes`
- `POST /api/v1/admin/metadata/taxonomies/:taxonomy_uuid/nodes`
- `PATCH /api/v1/admin/metadata/taxonomy-nodes/:node_uuid`
- `DELETE /api/v1/admin/metadata/taxonomy-nodes/:node_uuid`
- `GET /api/v1/admin/metadata/tags`
- `POST /api/v1/admin/metadata/tags`
- `GET /api/v1/admin/metadata/tag-bindings`
- `PUT /api/v1/admin/metadata/tag-bindings`
- `DELETE /api/v1/admin/metadata/tag-bindings`
- `GET /api/v1/admin/metadata/resource-types`
- `POST /api/v1/admin/metadata/resource-types`

查询接口必须支持：

- `tenant_uuid` 来自鉴权上下文，不允许请求方随意覆盖。
- `namespace`
- `module`
- `resource_type`
- `status`
- `q`
- `page`
- `page_size`

请求和响应 DTO 必须在 `specs/029-metadata-governance/contracts/http-openapi.yaml` 中明确，不允许只靠页面字段或数据库字段推导。

首版 DTO 至少包含：

- `CreateDictionaryNamespaceRequest`
- `UpdateDictionaryNamespaceRequest`
- `CreateDictionaryItemRequest`
- `UpdateDictionaryItemRequest`
- `CreateTaxonomyRequest`
- `UpdateTaxonomyRequest`
- `CreateTaxonomyNodeRequest`
- `MoveTaxonomyNodeRequest`
- `UpdateTaxonomyNodeRequest`
- `CreateTagRequest`
- `UpdateTagRequest`
- `MergeTagsRequest`
- `ReplaceTagBindingsRequest`
- `RegisterResourceTypeRequest`

统一响应：

- 列表接口使用 PowerX 统一分页 envelope。
- 单对象返回包含 `uuid`、机器标识、i18n map、当前 locale display 字段、状态和时间字段。
- 校验失败必须返回稳定错误码，不返回自由文本作为唯一错误依据。

## Capability 声明

底座能力按业务授权单元声明，不按每个 HTTP route 机械暴露：

- `com.corex.metadata.dictionary.read`
- `com.corex.metadata.dictionary.manage`
- `com.corex.metadata.taxonomy.read`
- `com.corex.metadata.taxonomy.manage`
- `com.corex.metadata.tag.read`
- `com.corex.metadata.tag.manage`
- `com.corex.metadata.resource_type.read`
- `com.corex.metadata.resource_type.manage`

每个 capability 必须包含：

- `permission_code`
- `agent_usable`
- `risk_level`
- `module`
- `display_name_i18n`
- `description_i18n`
- REST binding

permission_code 映射：

- `com.corex.metadata.dictionary.read` -> `metadata.dictionary:read`
- `com.corex.metadata.dictionary.manage` -> `metadata.dictionary:manage`
- `com.corex.metadata.taxonomy.read` -> `metadata.taxonomy:read`
- `com.corex.metadata.taxonomy.manage` -> `metadata.taxonomy:manage`
- `com.corex.metadata.tag.read` -> `metadata.tag:read`
- `com.corex.metadata.tag.manage` -> `metadata.tag:manage`
- `com.corex.metadata.resource_type.read` -> `metadata.resource_type:read`
- `com.corex.metadata.resource_type.manage` -> `metadata.resource_type:manage`

发布准入：

- `make capability-gen` 生成候选能力。
- `make capability-audit` 检查未声明路由和 required capability。
- `make capability-check` 同时校验生成候选与正式声明。

## 插件 Framework

framework 提供统一 client，不让插件直接拼底座内部路径：

```go
type MetadataClient interface {
    ResolveResourceType(ctx context.Context, resourceType string) (ResourceType, error)
    ListDictionaryItems(ctx context.Context, namespace string, query DictionaryQuery) ([]DictionaryItem, error)
    ListTaxonomyNodes(ctx context.Context, namespace string, query TaxonomyQuery) ([]TaxonomyNode, error)
    ListTags(ctx context.Context, resourceType string, query TagQuery) ([]Tag, error)
    ReplaceTagBindings(ctx context.Context, resourceType string, resourceUUID string, tagUUIDs []string) error
}
```

首版 framework 不提供管理类方法。插件如需创建 namespace、字典项、分类节点或标签，必须走 PowerX Admin API 并受租户管理员权限约束，不允许插件后台用 delegated service token 绕过管理权限。

delegated 模式：

- 通过 `/api/v1/tenant/invocations` 调用 metadata capability。
- payload 使用规范 REST 调用结构：`method`、`endpoint`、`query`、`body`。
- 网关根据 capability registry 路由到底座 REST binding。

local 模式：

- 使用插件本地开发存储。
- seed 必须来自同一份 canonical seed。
- 如果 seed 缺失，启动或初始化必须失败，不允许静默空列表。

## Seed 机制

canonical seed 存放在底座目录：

```text
backend/config/metadata_governance/
├── dictionaries.yaml
├── taxonomies.yaml
├── tags.yaml
└── resource_types.yaml
```

执行方式：

- Metadata seed 纳入显式 `make seed` / `cmd/database seed` 流程；低层 `metadata-seed` 命令仅用于单租户或单 seed 文件调试/修复。
- 不在 `cmd/app` 启动时自动 seed。
- 不在 AutoMigrate 中插入 seed 数据。
- tenant bootstrap 在租户创建 / 首次 upsert 流程中显式调用 seed service；失败时租户初始化返回失败。当前 TenantService 创建流程不是完整事务包裹，运维需按错误处理租户初始化结果，不允许后台启动时补跑隐式 seed。
- local 插件使用的 seed 必须从该目录复制或生成，缺失时 fail-fast。

## 事件与审计

建议事件：

- `metadata.dictionary.namespace.created`
- `metadata.dictionary.item.changed`
- `metadata.taxonomy.node.changed`
- `metadata.tag.changed`
- `metadata.tag.binding.changed`

审计字段：

- `tenant_uuid`
- `actor_uuid`
- `object_type`
- `object_uuid`
- `operation`
- `before`
- `after`
- `request_id`
