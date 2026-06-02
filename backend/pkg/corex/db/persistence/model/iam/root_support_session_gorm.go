package iam

import (
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

const (
	RootSupportSessionModeReadOnly     = "read_only"
	RootSupportSessionModeWriteEnabled = "write_enabled"

	RootSupportSessionStatusActive  = "active"
	RootSupportSessionStatusEnded   = "ended"
	RootSupportSessionStatusRevoked = "revoked"
)

type RootSupportSession struct {
	model.PowerModel

	RootUserID       uint64     `gorm:"column:root_user_id;not null;index:idx_root_support_root_user" json:"root_user_id"`
	TargetTenantUUID string     `gorm:"column:target_tenant_uuid;type:char(36);not null;index:idx_root_support_tenant_status,priority:1" json:"target_tenant_uuid"`
	Reason           string     `gorm:"column:reason;type:text;not null" json:"reason"`
	Mode             string     `gorm:"column:mode;type:varchar(32);not null;default:read_only;index" json:"mode"`
	Status           string     `gorm:"column:status;type:varchar(32);not null;default:active;index:idx_root_support_tenant_status,priority:2" json:"status"`
	StartedAt        time.Time  `gorm:"column:started_at;not null;index" json:"started_at"`
	EndedAt          *time.Time `gorm:"column:ended_at" json:"ended_at,omitempty"`
}

func (mdl *RootSupportSession) TableName() string {
	return model.PowerXSchema + "." + model.TableIAMRootSupportSession
}

func (mdl *RootSupportSession) GetTableName(needFull bool) string {
	if needFull {
		return mdl.TableName()
	}
	return model.TableIAMRootSupportSession
}
