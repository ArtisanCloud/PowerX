# Phase 1 数据建模

## 概述

本模块定义了 **MediaAsset** 数据模型，是 PowerX CoreX 内核中负责管理媒体资源（图片、视频、文件等）的核心实体。  
模型遵循 Spec-Kit 的 `dev_crud_http` 与 `dev_crud_grpc` 指南，并与 `constitution.md` 中的数据建模原则保持一致：
- 模块化、多租户、安全隔离
- 标准化审计字段
- 可扩展的 JSON 元数据
- 与 HTTP/gRPC 契约一致的字段命名和状态枚举

---

## 模型定义

```go
package media

import (
    "time"
    "gorm.io/gorm"
    "gorm.io/datatypes"
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
    coremodel.PowerUUIDModel

    TenantID   uint64         `gorm:"column:tenant_id;index"`
    Name       string         `gorm:"column:name;type:varchar(255);index"`
    Driver     string         `gorm:"column:driver;type:varchar(32);index"` // local/s3/...
    StorageKey string         `gorm:"column:storage_key;type:varchar(1024)"`
    Bucket     string         `gorm:"column:bucket;type:varchar(128);default:''"`
    BaseURL    string         `gorm:"column:base_url;type:varchar(512);default:''"`

    SizeBytes  int64          `gorm:"column:size_bytes;default:0"`
    MimeType   string         `gorm:"column:mime_type;type:varchar(128);default:''"`

    OwnerType  string         `gorm:"column:owner_type;type:varchar(32);default:'';index:idx_owner"`
    OwnerID    string         `gorm:"column:owner_id;type:varchar(64);default:'';index:idx_owner"`

    BusinessStatus string     `gorm:"column:business_status;type:varchar(32);default:'draft';index"`

    Tags       datatypes.JSON `gorm:"column:tags;type:jsonb;default:'[]'"`
    Meta       datatypes.JSON `gorm:"column:meta;type:jsonb;default:'{}'"`

    LastPresignedAt        *time.Time `gorm:"column:last_presigned_at"`
    LastPresignedTTLSeconds int32     `gorm:"column:last_presigned_ttl_seconds;default:43200"`
	
}

func (m *MediaAsset) TableName() string {
    return coremodel.PowerXSchema + "." + TableMediaAsset
}
````

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
| `Tags`                    | JSONB     | 标签数组                                             |
| `Meta`                    | JSONB     | 自定义元数据（结构化存储）                                    |
| `LastPresignedAt`         | time      | 最近生成预签名时间                                        |
| `LastPresignedTTLSeconds` | int32     | 预签名有效期（秒），默认 12 小时                               |
| `CreatedBy` / `UpdatedBy` | uint64    | 审计字段                                             |
| `DeletedAt`               | timestamp | 软删除标识                                            |

---

## 索引设计

| 索引名                                | 字段组合                              | 唯一性 | 说明            |
| ---------------------------------- | --------------------------------- | --- | ------------- |
| `uk_media_asset_tenant_driver_key` | (tenant_id, driver, storage_key)  | ✅   | 同一租户下相同驱动+键唯一 |
| `idx_media_asset_tenant_status`    | (tenant_id, business_status)      | ❌   | 常用业务状态过滤      |
| `idx_media_asset_owner`            | (tenant_id, owner_type, owner_id) | ❌   | 按归属筛选资源       |

---

## 迁移脚本

```go
func MigrateMediaModels(db *gorm.DB) error {
    if err := db.AutoMigrate(&MediaAsset{}); err != nil {
        return err
    }
    _ = db.Exec(`
        CREATE UNIQUE INDEX IF NOT EXISTS uk_media_asset_tenant_driver_key
        ON {{schema}}.media_assets (tenant_id, driver, storage_key)
        WHERE deleted_at IS NULL;
    `)
    _ = db.Exec(`
        CREATE INDEX IF NOT EXISTS idx_media_asset_tenant_status
        ON {{schema}}.media_assets (tenant_id, business_status);
    `)
    _ = db.Exec(`
        CREATE INDEX IF NOT EXISTS idx_media_asset_owner
        ON {{schema}}.media_assets (tenant_id, owner_type, owner_id);
    `)
    return nil
}
```

---

## 与 Spec 对齐映射

| Spec-Kit 模块             | 映射点                 | 说明                               |
| ----------------------- | ------------------- | -------------------------------- |
| constitution.md         | 模型需具备审计、可扩展性        | 已实现 CreatedBy / UpdatedBy / Meta |
| dev_crud_http_guides.md | CRUD 字段命名与 API 参数一致 | 字段命名遵循 HTTP 协议                   |
| dev_crud_grpc_guides.md | gRPC 契约字段保持同步       | 同步字段及状态枚举                        |
| dev_sts_guides.md       | STS 预签名管理           | LastPresignedAt / TTL 字段支持       |
| manifest.yaml           | 模块注册与 schema 名称保持一致 | TableName 指定 core schema         |

---

## 模块依赖

* `gorm.io/gorm`
* `gorm.io/datatypes`
* `pkg/corex/db/persistence/model`
* `cmd/database/migrate.go` (注册迁移)

---
