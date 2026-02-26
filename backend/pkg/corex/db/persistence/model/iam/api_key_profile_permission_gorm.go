package iam

import "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"

type APIKeyProfilePermission struct {
	ProfileID    uint64 `gorm:"column:profile_id;primaryKey" json:"profile_id"`
	PermissionID uint64 `gorm:"column:permission_id;primaryKey" json:"permission_id"`
	CreatedAt    int64  `gorm:"column:created_at;autoCreateTime:milli" json:"created_at"`
}

func (m *APIKeyProfilePermission) TableName() string {
	return model.PowerXSchema + "." + model.TableIAMAPIKeyProfilePermission
}

func (m *APIKeyProfilePermission) GetTableName(needFull bool) string {
	if needFull {
		return m.TableName()
	}
	return model.TableIAMAPIKeyProfilePermission
}
