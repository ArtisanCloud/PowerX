package audit

import (
	"time"

	"gorm.io/datatypes"
)

type Event struct {
	ID              string         `gorm:"type:uuid;primaryKey"`
	OccurredAt      time.Time      `gorm:"not null;index:idx_audit_tenant_time,priority:2,sort:desc"`
	TenantID        int64          `gorm:"not null;index:idx_audit_tenant_time,priority:1"`
	CorrelationID   string         `gorm:"type:text;index:idx_audit_corr"`
	Source          string         `gorm:"type:text;not null"`
	Operation       string         `gorm:"type:text;not null;index:idx_audit_op_outcome,priority:2"`
	ResourceType    string         `gorm:"type:text;not null;index:idx_audit_resource,priority:1"`
	ResourceID      string         `gorm:"type:text;not null;index:idx_audit_resource,priority:2"`
	ResourceName    string         `gorm:"type:text"`
	Outcome         string         `gorm:"type:text;not null;index:idx_audit_op_outcome,priority:3"`
	Severity        string         `gorm:"type:text;not null"`
	ActorUserID     *int64         `gorm:"index:idx_audit_actor"`
	ActorUsername   string         `gorm:"type:text"`
	ActorDisplay    string         `gorm:"type:text"`
	ActorRoleIDs    datatypes.JSON `gorm:"type:jsonb"`
	ClientIP        string         `gorm:"type:inet"`
	ClientUA        string         `gorm:"type:text"`
	ChangesBefore   datatypes.JSON `gorm:"type:jsonb"`
	ChangesAfter    datatypes.JSON `gorm:"type:jsonb"`
	ChangesDiff     datatypes.JSON `gorm:"type:jsonb"`
	ChangesRedacted bool           `gorm:"default:false"`
	Meta            datatypes.JSON `gorm:"type:jsonb"`
	Hash            string         `gorm:"type:text"`
	PrevHash        string         `gorm:"type:text"`
}

func (Event) TableName() string { return "audit_event" }
