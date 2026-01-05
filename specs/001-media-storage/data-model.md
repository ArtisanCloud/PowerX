# Phase 1 数据建模

## 概述

本模块定义 **MediaAsset** 数据模型，是 PowerX CoreX 内核中负责管理媒体资源（图片、视频、文件等）的核心实体。
模型遵循 Spec-Kit 的 `dev_crud_http` 与 `dev_crud_grpc` 指南，并与 `constitution.md` 中的数据建模原则保持一致：

* 模块化、多租户、安全隔离
* 标准化审计字段
* 可扩展的 JSON 元数据
* 与 HTTP/gRPC 契约一致的字段命名和状态枚举

---

## 模型定义（含索引标签）

```go
package media

import (
    "time"

    "gorm.io/datatypes"
    "gorm.io/gorm"

    coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

const (
    MediaAssetStatusDraft       = "draft"
    MediaAssetStatusUnderReview = "under_review"
    MediaAssetStatusPublished   = "published"
    MediaAssetStatusArchived    = "archived"

    TableMediaAsset = "media_assets"
)

type MediaAsset struct {
    coremodel.PowerUUIDModel // 假定内含 ID/CreatedAt/UpdatedAt/DeletedAt/CreatedBy/UpdatedBy 等

    // 复合唯一（部分索引）：tenant_id + driver + storage_key，软删后可重用（WHERE deleted_at IS NULL）
    TenantID   uint64 `gorm:"column:tenant_id;index:uk_media_asset_tenant_driver_key,unique,where:deleted_at IS NULL; index:idx_media_asset_tenant_status,priority:1; index:idx_media_asset_owner,priority:1"`
    Name       string `gorm:"column:name;type:varchar(255);index"`
    Driver     string `gorm:"column:driver;type:varchar(32);index:uk_media_asset_tenant_driver_key,unique,where:deleted_at IS NULL"`
    StorageKey string `gorm:"column:storage_key;type:varchar(1024);index:uk_media_asset_tenant_driver_key,unique,where:deleted_at IS NULL"`

    Bucket  string `gorm:"column:bucket;type:varchar(128);default:''"`
    BaseURL string `gorm:"column:base_url;type:varchar(512);default:''"`

    SizeBytes int64  `gorm:"column:size_bytes;default:0"`
    MimeType  string `gorm:"column:mime_type;type:varchar(128);default:''"`

    // 归属筛选复合索引：tenant_id, owner_type, owner_id
    OwnerType string `gorm:"column:owner_type;type:varchar(32);default:'';index:idx_media_asset_owner,priority:2"`
    OwnerID   string `gorm:"column:owner_id;type:varchar(64);default:'';index:idx_media_asset_owner,priority:3"`

    // 状态过滤复合索引：tenant_id, business_status
    BusinessStatus string `gorm:"column:business_status;type:varchar(32);default:'draft';index:idx_media_asset_tenant_status,priority:2"`

    // GIN 索引：jsonb
    Tags datatypes.JSON `gorm:"column:tags;type:jsonb;default:'[]';index:idx_media_asset_tags_gin,using:gin"`
    Meta datatypes.JSON `gorm:"column:meta;type:jsonb;default:'{}'"`

    LastPresignedAt         *time.Time `gorm:"column:last_presigned_at"`
    LastPresignedTTLSeconds int32      `gorm:"column:last_presigned_ttl_seconds;default:43200"`
}

func (m *MediaAsset) TableName() string {
    return coremodel.PowerXSchema + "." + TableMediaAsset
}
```

---

## 字段说明

| 字段名                       | 类型        | 说明                                               |
| ------------------------- | --------- | ------------------------------------------------ |
| `TenantID`                | uint64    | 所属租户，用于多租户数据隔离                                   |
| `Driver`                  | string    | 存储驱动类型（local、s3 等）                               |
| `StorageKey`              | string    | 对象存储键（路径或 Key）                                   |
| `Bucket`                  | string    | 存储桶名称                                            |
| `BaseURL`                 | string    | 访问基地址（CDN 或本地 URL）                               |
| `SizeBytes`               | int64     | 文件大小（字节）                                         |
| `MimeType`                | string    | 文件 MIME 类型                                       |
| `OwnerType` / `OwnerID`   | string    | 文件归属主体（用户、团队、应用）                                 |
| `BusinessStatus`          | string    | 业务状态：draft / under_review / published / archived |
| `Tags`                    | JSONB     | 标签数组（GIN 索引）                                     |
| `Meta`                    | JSONB     | 自定义元数据（结构化存储）                                    |
| `LastPresignedAt`         | time      | 最近生成预签名时间                                        |
| `LastPresignedTTLSeconds` | int32     | 预签名有效期（秒），默认 12 小时                               |
| `CreatedBy` / `UpdatedBy` | uint64    | 审计字段（由内嵌基类提供）                                    |
| `DeletedAt`               | timestamp | 软删除标识（由内嵌基类提供）                                   |

---

## 索引设计（由模型标签自动生成）

| 索引名                                | 字段组合                              | 唯一性 | 说明                                           |
| ---------------------------------- | --------------------------------- | --- | -------------------------------------------- |
| `uk_media_asset_tenant_driver_key` | (tenant_id, driver, storage_key)  | ✅   | **部分唯一索引**：`WHERE deleted_at IS NULL`，软删后可重用 |
| `idx_media_asset_tenant_status`    | (tenant_id, business_status)      | ❌   | 常用业务状态过滤                                     |
| `idx_media_asset_owner`            | (tenant_id, owner_type, owner_id) | ❌   | 归属主体筛选                                       |
| `idx_media_asset_tags_gin`         | (tags)（jsonb，GIN）                 | ❌   | 标签包含/交集查询优化（GIN）                             |

> 以上索引均**通过 GORM 结构体标签声明**，在迁移时由 Migrator 自动创建；禁止手写 SQL。

---

## 迁移策略

* **CoreX（内核，含 Media Assets Management）**：纳入 `pkg/corex/db/migration.go` 的 `MigrateCoreModels(db *gorm.DB)`，仅通过 `db.AutoMigrate(&media.MediaAsset{}` …)` 注册模型。
* **禁止**提交或在迁移中生成任何 `.sql` 文件，**禁止**在迁移中执行 `db.Exec(...)`。
* 可选运行期校验可使用 `db.Migrator().HasIndex/HasConstraint`，但不得引入原生 SQL。

### 迁移示例（CoreX 统一入口）

```go
// pkg/corex/db/migration.go
package corexdb

import (
    "gorm.io/gorm"
    mediamodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/media"
)

func MigrateCoreModels(db *gorm.DB) error {
    return db.AutoMigrate(
        &mediamodel.MediaAsset{},
        // 其他 CoreX 内核模型...
    )
}
```

> 独立 Server 模块（例如 Agent）在 `cmd/database/migrate.go` 中实现并调用自己的 `MigrateAgentModels`；**Media Assets Management 属于 CoreX 内核**，因此不单独提供 `MigrateMediaModels`。

---

## 与 Spec 对齐映射

| Spec-Kit 模块             | 映射点             | 说明                                            |
| ----------------------- | --------------- | --------------------------------------------- |
| constitution.md         | 审计、可扩展性、多租户隔离   | 基类审计 + JSONB 元数据 + TenantID                   |
| dev_crud_http_guides.md | 字段命名与 API 参数一致  | 字段命名遵循 HTTP 契约                                |
| dev_crud_grpc_guides.md | 与 gRPC 契约字段同步   | 统一的字段与状态枚举                                    |
| dev_sts_guides.md       | STS 预签名管理       | `LastPresignedAt` / `LastPresignedTTLSeconds` |
| manifest.yaml           | 模块注册与 schema 一致 | `TableName` 指定 core schema                    |

---

## 模块依赖

* `gorm.io/gorm`
* `gorm.io/datatypes`
* `pkg/corex/db/persistence/model`
* `pkg/corex/db/migration.go`（CoreX 统一迁移入口）
