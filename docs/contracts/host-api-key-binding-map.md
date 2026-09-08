# Host API Key 精确授权映射

2026-09-08：按正式 capability YAML 中显式 api_key 元数据生成；这是权限定义，不自动授予任何 Key。tenant registration、具体 Gateway Key 的实际授权仍由各 Host Service 校验。grant-status 使用同一 scope/action/resource 匹配函数，不从 profile 推断授权。

| Method | Path | Capability | Scope | Action | Resource type/pattern |
| --- | --- | --- | --- | --- | --- |
| `POST` | `/api/v1/tenant/capabilities:grant-status` | `com.corex.capabilities.grant_status.read` | `_scope.capabilities.grant_status.read` | `read` | `api/grant-status` |
| `GET` | `/api/v1/tenant/iam/members/{member_uuid}` | `com.corex.iam.members.read` | `_scope.iam.members.directory.read` | `read` | `api/directory-members` |
| `POST` | `/api/v1/tenant/iam/members:batch-get` | `com.corex.iam.members.read` | `_scope.iam.members.directory.read` | `read` | `api/directory-members` |
| `GET` | `/api/v1/tenant/iam/tenant` | `com.corex.iam.directory.read` | `_scope.iam.directory.catalog.read` | `read` | `api/directory-catalog` |
| `GET` | `/api/v1/tenant/iam/members` | `com.corex.iam.directory.read` | `_scope.iam.directory.catalog.read` | `read` | `api/directory-catalog` |
| `POST` | `/api/v1/tenant/iam/members:batch-resolve` | `com.corex.iam.directory.read` | `_scope.iam.directory.catalog.read` | `read` | `api/directory-catalog` |
| `POST` | `/api/v1/tenant/iam/members:batch-find-by-display-names` | `com.corex.iam.directory.read` | `_scope.iam.directory.catalog.read` | `read` | `api/directory-catalog` |
| `GET` | `/api/v1/tenant/iam/departments` | `com.corex.iam.directory.read` | `_scope.iam.directory.catalog.read` | `read` | `api/directory-catalog` |
| `GET` | `/api/v1/tenant/iam/roles` | `com.corex.iam.directory.read` | `_scope.iam.directory.catalog.read` | `read` | `api/directory-catalog` |
| `GET` | `/api/v1/tenant/iam/permissions` | `com.corex.iam.directory.read` | `_scope.iam.directory.catalog.read` | `read` | `api/directory-catalog` |
| `POST` | `/api/v1/tenant/iam/authorization:check` | `com.corex.iam.authorization.check` | `_scope.iam.authorization.check` | `read` | `api/authorization-check` |
| `GET` | `/api/v1/tenant/knowledge/spaces` | `com.corex.knowledge.directory.read` | `_scope.knowledge.directory.read` | `read` | `api/directory` |
| `POST` | `/api/v1/tenant/knowledge/search` | `com.corex.knowledge.search.read` | `_scope.knowledge.search.read` | `read` | `api/search` |
| `POST` | `/api/v1/tenant/knowledge/spaces/{space_uuid}/documents` | `com.corex.knowledge.document.manage` | `_scope.knowledge.document.manage` | `manage` | `api/document` |
| `DELETE` | `/api/v1/tenant/knowledge/spaces/{space_uuid}/documents/{document_uuid}` | `com.corex.knowledge.document.manage` | `_scope.knowledge.document.manage` | `manage` | `api/document` |
| `POST` | `/api/v1/tenant/knowledge/spaces/{space_uuid}/indexes:rebuild` | `com.corex.knowledge.document.manage` | `_scope.knowledge.document.manage` | `manage` | `api/document` |
| `GET` | `/api/v1/tenant/knowledge/index-jobs/{job_uuid}` | `com.corex.knowledge.document.manage` | `_scope.knowledge.document.manage` | `manage` | `api/document` |
| `GET` | `/api/v1/tenant/media/assets` | `com.corex.media.assets.read` | `_scope.media.assets.read` | `read` | `api/assets` |
| `GET` | `/api/v1/tenant/media/assets/{asset_uuid}` | `com.corex.media.assets.read` | `_scope.media.assets.read` | `read` | `api/assets` |
| `GET` | `/api/v1/tenant/media/assets/variants/{variant_uuid}` | `com.corex.media.assets.read` | `_scope.media.assets.read` | `read` | `api/assets` |
| `POST` | `/api/v1/tenant/media/assets` | `com.corex.media.assets.manage` | `_scope.media.assets.manage` | `manage` | `api/assets` |
| `PATCH` | `/api/v1/tenant/media/assets/{asset_uuid}` | `com.corex.media.assets.manage` | `_scope.media.assets.manage` | `manage` | `api/assets` |
| `DELETE` | `/api/v1/tenant/media/assets/{asset_uuid}` | `com.corex.media.assets.manage` | `_scope.media.assets.manage` | `manage` | `api/assets` |
| `POST` | `/api/v1/tenant/media/assets/{asset_uuid}/presign-upload` | `com.corex.media.assets.transfer` | `_scope.media.assets.transfer` | `transfer` | `api/assets` |
| `POST` | `/api/v1/tenant/media/assets/{asset_uuid}/complete-upload` | `com.corex.media.assets.transfer` | `_scope.media.assets.transfer` | `transfer` | `api/assets` |
| `POST` | `/api/v1/tenant/media/assets/{asset_uuid}/presign-download` | `com.corex.media.assets.transfer` | `_scope.media.assets.transfer` | `transfer` | `api/assets` |
| `POST` | `/api/v1/tenant/media/assets/{asset_uuid}/variants` | `com.corex.media.assets.variants.manage` | `_scope.media.assets.variants.manage` | `manage` | `api/variants` |
| `GET` | `/api/v1/tenant/metadata/dictionaries` | `com.corex.metadata.dictionary.read` | `_scope.metadata.dictionary.read` | `read` | `api/dictionary` |
| `GET` | `/api/v1/tenant/metadata/dictionaries/{namespace_uuid}/items` | `com.corex.metadata.dictionary.read` | `_scope.metadata.dictionary.read` | `read` | `api/dictionary` |
| `POST` | `/api/v1/tenant/metadata/dictionaries` | `com.corex.metadata.dictionary.manage` | `_scope.metadata.dictionary.manage` | `manage` | `api/dictionary` |
| `PATCH` | `/api/v1/tenant/metadata/dictionaries/{namespace_uuid}` | `com.corex.metadata.dictionary.manage` | `_scope.metadata.dictionary.manage` | `manage` | `api/dictionary` |
| `POST` | `/api/v1/tenant/metadata/dictionaries/{namespace_uuid}/items` | `com.corex.metadata.dictionary.manage` | `_scope.metadata.dictionary.manage` | `manage` | `api/dictionary` |
| `PATCH` | `/api/v1/tenant/metadata/dictionary-items/{item_uuid}` | `com.corex.metadata.dictionary.manage` | `_scope.metadata.dictionary.manage` | `manage` | `api/dictionary` |
| `GET` | `/api/v1/tenant/metadata/taxonomies` | `com.corex.metadata.taxonomy.read` | `_scope.metadata.taxonomy.read` | `read` | `api/taxonomy` |
| `GET` | `/api/v1/tenant/metadata/taxonomies/{taxonomy_uuid}/nodes` | `com.corex.metadata.taxonomy.read` | `_scope.metadata.taxonomy.read` | `read` | `api/taxonomy` |
| `POST` | `/api/v1/tenant/metadata/taxonomies` | `com.corex.metadata.taxonomy.manage` | `_scope.metadata.taxonomy.manage` | `manage` | `api/taxonomy` |
| `POST` | `/api/v1/tenant/metadata/taxonomies/{taxonomy_uuid}/nodes` | `com.corex.metadata.taxonomy.manage` | `_scope.metadata.taxonomy.manage` | `manage` | `api/taxonomy` |
| `PATCH` | `/api/v1/tenant/metadata/taxonomy-nodes/{node_uuid}` | `com.corex.metadata.taxonomy.manage` | `_scope.metadata.taxonomy.manage` | `manage` | `api/taxonomy` |
| `GET` | `/api/v1/tenant/metadata/tags` | `com.corex.metadata.tag.read` | `_scope.metadata.tag.read` | `read` | `api/tag` |
| `POST` | `/api/v1/tenant/metadata/tags` | `com.corex.metadata.tag.manage` | `_scope.metadata.tag.manage` | `manage` | `api/tag` |
| `POST` | `/api/v1/tenant/metadata/tag-bindings` | `com.corex.metadata.tag.manage` | `_scope.metadata.tag.manage` | `manage` | `api/tag` |
| `DELETE` | `/api/v1/tenant/metadata/tag-bindings/{binding_uuid}` | `com.corex.metadata.tag.manage` | `_scope.metadata.tag.manage` | `manage` | `api/tag` |
| `GET` | `/api/v1/tenant/metadata/resource-types` | `com.corex.metadata.resource_type.read` | `_scope.metadata.resource_type.read` | `read` | `api/resource_type` |
| `POST` | `/api/v1/tenant/metadata/resource-types` | `com.corex.metadata.resource_type.manage` | `_scope.metadata.resource_type.manage` | `manage` | `api/resource_type` |
| `PATCH` | `/api/v1/tenant/metadata/resource-types/{resource_type_uuid}` | `com.corex.metadata.resource_type.manage` | `_scope.metadata.resource_type.manage` | `manage` | `api/resource_type` |
| `GET` | `/api/v1/tenant/plugin-release/install-sessions/{session_uuid}` | `com.corex.plugin_release.sessions.read` | `_scope.plugin_release.sessions.read` | `read` | `api/sessions` |
| `POST` | `/api/v1/tenant/plugin-release/install-sessions` | `com.corex.plugin_release.sessions.manage` | `_scope.plugin_release.sessions.manage` | `manage` | `api/sessions` |
| `POST` | `/api/v1/tenant/plugin-release/install-sessions/{session_uuid}/stop` | `com.corex.plugin_release.sessions.manage` | `_scope.plugin_release.sessions.manage` | `manage` | `api/sessions` |
| `GET` | `/api/v1/tenant/plugin-release/import-jobs/{job_uuid}` | `com.corex.plugin_release.imports.read` | `_scope.plugin_release.imports.read` | `read` | `api/imports` |
| `POST` | `/api/v1/tenant/plugin-release/import-jobs` | `com.corex.plugin_release.imports.manage` | `_scope.plugin_release.imports.manage` | `manage` | `api/imports` |

授权位置：IAM directory_access_service.go；Metadata host_contract_access.go；Media host_contract_access.go；Plugin Release openapi/plugin_release/host_contract_access.go；Knowledge 的 tenant Host service；grant-status 的 grant_status_service.go。共同匹配实现：IntegrationGatewayAPIKeyPermissionRepository.HasPermission / APIKeyPermissionGranted。

验证：apikeypermissions/TestCheckedInPlatformCatalogExplicitAPIKeyMappings 检查正式权限生成；TestLiveAPIKeyGrantStatusMatchesActualHost 使用真实本地 HTTP 验证具体 key 授权/撤销与 IAM 一致。Session 本轮只支持 STS，不添加 API Key 绑定。未列出的动态插件能力、队列/总线静态 runtime 操作不能据本表推断支持。
