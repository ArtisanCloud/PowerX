package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

const (
	TableAgentShareRecord = "agent_shares"
)

// AgentShareRecord 保存代理跨租户共享记录。
type AgentShareRecord struct {
	coremodel.PowerUUIDModel

	AgentUUID       uuid.UUID      `gorm:"column:agent_uuid;type:uuid;not null;index:idx_agent_share_agent,priority:1;uniqueIndex:idx_agent_share_unique,priority:1" json:"agent_uuid"`
	TargetTenantUUID string         `gorm:"column:target_tenant_uuid;type:varchar(128);not null;index:idx_agent_share_agent,priority:2;index:idx_agent_share_tenant;uniqueIndex:idx_agent_share_unique,priority:2" json:"target_tenant_uuid"`
	Status          string         `gorm:"column:status;type:varchar(32);not null;index:idx_agent_share_status" json:"status"`
	Quotas          datatypes.JSON `gorm:"column:quotas;type:jsonb;default:'[]'" json:"quotas"`
	Metadata        datatypes.JSON `gorm:"column:metadata;type:jsonb;default:'{}'" json:"metadata"`
	IssuedBy        string         `gorm:"column:issued_by;type:varchar(128)" json:"issued_by"`
	ValidatedAt     *time.Time     `gorm:"column:validated_at" json:"validated_at,omitempty"`
	ProvisionedAt   *time.Time     `gorm:"column:provisioned_at" json:"provisioned_at,omitempty"`
	NextReviewAt    *time.Time     `gorm:"column:next_review_at" json:"next_review_at,omitempty"`
	RevokedBy       string         `gorm:"column:revoked_by;type:varchar(128)" json:"revoked_by,omitempty"`
	RevokedAt       *time.Time     `gorm:"column:revoked_at" json:"revoked_at,omitempty"`
	RevokeReason    string         `gorm:"column:revoke_reason;type:text" json:"revoke_reason,omitempty"`
	ValidationFail  bool           `gorm:"column:validation_failed;default:false" json:"validation_failed"`
	ValidationError string         `gorm:"column:validation_error;type:text" json:"validation_error,omitempty"`
}

func (AgentShareRecord) TableName() string {
	return coremodel.PowerXSchema + "." + TableAgentShareRecord
}
