package knowledge

import (
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/datatypes"
)

const (
	ProfileStatusDraft     = "draft"
	ProfileStatusPublished = "published"
	ProfileStatusArchived  = "archived"
)

// IngestionProfileVersion describes ingestion-time strategy configuration (versioned).
type IngestionProfileVersion struct {
	coremodel.PowerUUIDModel

	TenantUUID     string         `gorm:"column:tenant_uuid;type:varchar(128);not null;index:idx_ing_profile_tenant_key;uniqueIndex:uk_ing_profile_version,priority:1" json:"tenant_uuid"`
	ProfileKey     string         `gorm:"column:profile_key;type:varchar(128);not null;uniqueIndex:uk_ing_profile_version,priority:2;index:idx_ing_profile_key" json:"profile_key"`
	Version        int            `gorm:"column:version;type:int;not null;default:1;uniqueIndex:uk_ing_profile_version,priority:3" json:"version"`
	Status         string         `gorm:"column:status;type:varchar(32);not null;default:'draft';index:idx_ing_profile_status" json:"status"`
	DisplayName    string         `gorm:"column:display_name;type:varchar(128);not null" json:"display_name"`
	Config         datatypes.JSON `gorm:"column:config;type:jsonb;not null;default:'{}'" json:"config"`
	RollbackFromID uint64         `gorm:"column:rollback_from_id;type:bigint" json:"rollback_from_id"`
	PublishedAt    *time.Time     `gorm:"column:published_at" json:"published_at,omitempty"`
	PublishedBy    string         `gorm:"column:published_by;type:varchar(128)" json:"published_by,omitempty"`
	CreatedBy      string         `gorm:"column:created_by;type:varchar(128)" json:"created_by,omitempty"`
}

func (IngestionProfileVersion) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableKnowledgeIngestionProfileVersions
}

// IndexProfileVersion describes index-time strategy configuration (versioned).
type IndexProfileVersion struct {
	coremodel.PowerUUIDModel

	TenantUUID     string         `gorm:"column:tenant_uuid;type:varchar(128);not null;index:idx_index_profile_tenant_key;uniqueIndex:uk_index_profile_version,priority:1" json:"tenant_uuid"`
	ProfileKey     string         `gorm:"column:profile_key;type:varchar(128);not null;uniqueIndex:uk_index_profile_version,priority:2;index:idx_index_profile_key" json:"profile_key"`
	Version        int            `gorm:"column:version;type:int;not null;default:1;uniqueIndex:uk_index_profile_version,priority:3" json:"version"`
	Status         string         `gorm:"column:status;type:varchar(32);not null;default:'draft';index:idx_index_profile_status" json:"status"`
	DisplayName    string         `gorm:"column:display_name;type:varchar(128);not null" json:"display_name"`
	Config         datatypes.JSON `gorm:"column:config;type:jsonb;not null;default:'{}'" json:"config"`
	RollbackFromID uint64         `gorm:"column:rollback_from_id;type:bigint" json:"rollback_from_id"`
	PublishedAt    *time.Time     `gorm:"column:published_at" json:"published_at,omitempty"`
	PublishedBy    string         `gorm:"column:published_by;type:varchar(128)" json:"published_by,omitempty"`
	CreatedBy      string         `gorm:"column:created_by;type:varchar(128)" json:"created_by,omitempty"`
}

func (IndexProfileVersion) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableKnowledgeIndexProfileVersions
}

// RAGProfileVersion describes online retrieval strategy configuration (versioned).
type RAGProfileVersion struct {
	coremodel.PowerUUIDModel

	TenantUUID     string         `gorm:"column:tenant_uuid;type:varchar(128);not null;index:idx_rag_profile_tenant_key;uniqueIndex:uk_rag_profile_version,priority:1" json:"tenant_uuid"`
	ProfileKey     string         `gorm:"column:profile_key;type:varchar(128);not null;uniqueIndex:uk_rag_profile_version,priority:2;index:idx_rag_profile_key" json:"profile_key"`
	Version        int            `gorm:"column:version;type:int;not null;default:1;uniqueIndex:uk_rag_profile_version,priority:3" json:"version"`
	Status         string         `gorm:"column:status;type:varchar(32);not null;default:'draft';index:idx_rag_profile_status" json:"status"`
	DisplayName    string         `gorm:"column:display_name;type:varchar(128);not null" json:"display_name"`
	Config         datatypes.JSON `gorm:"column:config;type:jsonb;not null;default:'{}'" json:"config"`
	RollbackFromID uint64         `gorm:"column:rollback_from_id;type:bigint" json:"rollback_from_id"`
	PublishedAt    *time.Time     `gorm:"column:published_at" json:"published_at,omitempty"`
	PublishedBy    string         `gorm:"column:published_by;type:varchar(128)" json:"published_by,omitempty"`
	CreatedBy      string         `gorm:"column:created_by;type:varchar(128)" json:"created_by,omitempty"`
}

func (RAGProfileVersion) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableKnowledgeRAGProfileVersions
}

