# Metadata Governance 机制设计

## 核心对象

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

查询接口必须支持：

- `tenant_uuid` 来自鉴权上下文，不允许请求方随意覆盖。
- `namespace`
- `module`
- `resource_type`
- `status`
- `q`
- `page`
- `page_size`

## Capability 声明

底座能力按业务授权单元声明，不按每个 HTTP route 机械暴露：

- `com.corex.metadata.dictionary.read`
- `com.corex.metadata.dictionary.manage`
- `com.corex.metadata.taxonomy.read`
- `com.corex.metadata.taxonomy.manage`
- `com.corex.metadata.tag.read`
- `com.corex.metadata.tag.manage`

每个 capability 必须包含：

- `permission_code`
- `agent_usable`
- `risk_level`
- `module`
- `display_name_i18n`
- `description_i18n`
- REST binding

发布准入：

- `make capability-gen` 生成候选能力。
- `make capability-audit` 检查未声明路由和 required capability。
- `make capability-check` 同时校验生成候选与正式声明。

## 插件 Framework

framework 提供统一 client，不让插件直接拼底座内部路径：

```go
type MetadataClient interface {
    ListDictionaryItems(ctx context.Context, namespace string, query DictionaryQuery) ([]DictionaryItem, error)
    ListTaxonomyNodes(ctx context.Context, namespace string, query TaxonomyQuery) ([]TaxonomyNode, error)
    ListTags(ctx context.Context, resourceType string, query TagQuery) ([]Tag, error)
    ReplaceTagBindings(ctx context.Context, resourceType string, resourceUUID string, tagUUIDs []string) error
}
```

delegated 模式：

- 通过 `/api/v1/tenant/invocations` 调用 metadata capability。
- payload 使用规范 REST 调用结构：`method`、`endpoint`、`query`、`body`。
- 网关根据 capability registry 路由到底座 REST binding。

local 模式：

- 使用插件本地开发存储。
- seed 必须来自同一份 canonical seed。
- 如果 seed 缺失，启动或初始化必须失败，不允许静默空列表。

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
