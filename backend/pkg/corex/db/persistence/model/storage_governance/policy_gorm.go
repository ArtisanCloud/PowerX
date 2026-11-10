// Package storagegovmodel 保存平台级观测与策略草稿；TenantID=0 为平台域，不能当租户通配符。
package storagegovmodel

import (
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/datatypes"
)

// Policy 按部署实例保存草稿，不承担跨数据库的全局预算协调。
type Policy struct {
	coremodel.PowerModel
	TenantID      uint64         `gorm:"not null;default:0;uniqueIndex:uk_storage_policy_instance"`
	InstanceID    string         `gorm:"type:varchar(128);not null;uniqueIndex:uk_storage_policy_instance"`
	Revision      int64          `gorm:"not null"`
	SchemaVersion int            `gorm:"not null"`
	Document      datatypes.JSON `gorm:"type:jsonb;not null"`
	UpdatedBy     uint64         `gorm:"not null"`
	Status        int16          `gorm:"not null;default:1"`
}

func (*Policy) TableName() string { return coremodel.PowerXSchema + "." + TablePolicy }

// PolicyRevision 与策略 CAS 更新在同一事务内写入，保留最近 100 版修改审计。
type PolicyRevision struct {
	coremodel.PowerModel
	TenantID   uint64         `gorm:"not null;default:0;uniqueIndex:uk_storage_policy_revision"`
	InstanceID string         `gorm:"type:varchar(128);not null;uniqueIndex:uk_storage_policy_revision"`
	Revision   int64          `gorm:"not null;uniqueIndex:uk_storage_policy_revision"`
	Document   datatypes.JSON `gorm:"type:jsonb;not null"`
	ActorID    uint64         `gorm:"not null"`
	Status     int16          `gorm:"not null;default:1"`
}

func (*PolicyRevision) TableName() string { return coremodel.PowerXSchema + "." + TablePolicyRevision }
