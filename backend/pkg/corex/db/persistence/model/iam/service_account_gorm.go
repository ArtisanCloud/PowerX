package iam

import "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"

type APIKeyProfile struct {
	ID            uint64  `gorm:"primaryKey"`
	TenantUUID    string  `gorm:"column:tenant_uuid;type:char(36);index;not null"`
	OwnerMemberID *uint64 `gorm:"column:owner_member_id;index" json:"owner_member_id,omitempty"`
	Key           string  `gorm:"type:varchar(64);not null;index:,unique,composite:uk_svc_tenant_key"`
	Name          string  `gorm:"type:varchar(128);not null"`
	Status        int16   `gorm:"index;default:1"`
}

func (m *APIKeyProfile) TableName() string {
	return model.PowerXSchema + "." + model.TableIAMAPIKeyProfile
}

func (m *APIKeyProfile) GetTableName(needFull bool) string {
	if needFull {
		return m.TableName()
	}
	return model.TableIAMAPIKeyProfile
}
