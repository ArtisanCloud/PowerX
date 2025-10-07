# Phase 1 Data Model — Media Asset Admin Capabilities

**Source Spec**: [/private/var/www/html/ArtisanCloud/X/PowerX/Core/PowerX/specs/001-docs-media-storage/spec.md](spec.md)  
**Research Inputs**: [/private/var/www/html/ArtisanCloud/X/PowerX/Core/PowerX/specs/001-docs-media-storage/research.md](research.md)

## Entities

### MediaAsset (persistent)
- **Identity**: UUID (`id`)，同时要求 `tenant_id + driver + object_key` 唯一。
- **Core Fields**:
  - `tenant_id` (string/UUID)：租户隔离标识。
  - `driver` (string, 32chars)：local/s3 等驱动枚举。
  - `bucket` (string, 128chars)：对象存储桶或本地根。
  - `object_key` (string, 512chars)：驱动内唯一路径。
  - `file_name` (string, 256chars)：原始文件名。
  - `content_type` (string, 128chars)。
  - `size` (int64)：字节数。
  - `folder` (string, 256chars)：业务目录/命名空间。
  - `tags` (JSON array[string])：业务标签。
  - `tag_names` (virtual generated string)：排序后的标签串用于索引。
  - `owner_type` (string, 64chars, nullable)。
  - `owner_id` (uint64, nullable)。
  - `meta` (JSON object)：附加元数据。
  - `status` (enum int8)：0=Draft,1=UnderReview,2=Published,3=Archived。
  - `created_by` / `updated_by` (uint64)：操作人。
  - `created_at`, `updated_at`, `deleted_at` (timestamp, 软删)。
- **Validation Rules**:
  - `driver` 必须存在并启用。
  - 上传时必须提供 `file` 或 `file_url`。
  - `size` >= 0；`tags` 数量 ≤ 20。
  - `status` 仅允许在 0-3 枚举中。
- **State Transitions**:
  - `Draft → UnderReview/Published`：运营审核流程。
  - `Published → Archived`：资源下线。
  - 软删后进入 `deleted_at` 非空状态，等待定时清理。

### PresignToken (transient)
- **Purpose**: 生成上传/下载临时授权的响应模型（非持久化）。
- **Fields**:
  - `asset_id` (UUID，可空：预上传场景)。
  - `method` (string)：GET/PUT。
  - `url` (string)。
  - `expire_at` (timestamp)：默认 12 小时内。
  - `form_fields` (map[string]string，可选)。
- **Constraints**:
  - 仅授权管理员可请求。
  - `expire_at` 不超过配置的最大值（默认 12h，可配置）。

### StorageDriverConfig (config-bound)
- **Purpose**: 通过 `config/storage.go` 注入的驱动定义。
- **Fields**:
  - `id` (string)。
  - `type` (`local`/`s3` 等)。
  - `enabled` (bool)。
  - `endpoint`, `bucket`, `root`, `base_url`, `region`, `use_ssl`, `path_style` 等具体属性。
  - `presign_expiration` (duration)：默认 12h，可覆盖。
- **Notes**: 驱动配置不入库，由配置文件或远程配置中心管理。

## Relationships
- `MediaAsset` 按 `tenant_id` 与其它业务实体（owner_type/owner_id）关联，通过外部查询关联业务域。
- `MediaAsset` 与 `StorageDriverConfig` 在运行时通过 `driver` 字段匹配，无数据库外键。
- 预签名流程使用 `MediaAsset`（如 asset_id 非空）验证状态，确保仅 Published 或 Draft（权责）资源可签名。

## Derived / Generated Data
- `tag_names`：`tags` JSON 数组经过排序、`|` 拼接的虚拟列，便于 LIKE 索引。
- 审计日志（另存于 audit 表）记录操作人和操作类型，与 `MediaAsset` 通过 asset_id 关联。

## Open Questions（待任务阶段落地）
- 是否需要 owner_type/owner_id 的组合索引取决于查询频率，将在实现时根据真实业务评估。
