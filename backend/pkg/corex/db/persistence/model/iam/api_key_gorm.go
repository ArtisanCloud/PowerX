// pkg/corex/db/persistence/model/iam/api_key_gorm.go
package iam

import "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"

type APIKey struct {
	ID           uint64 `gorm:"primaryKey"`
	TenantUUID   string `gorm:"column:tenant_uuid;type:char(36);index;not null"`
	ProfileID    uint64 `gorm:"column:profile_id;index;not null"`
	KeyHash      string `gorm:"type:varchar(128);not null;uniqueIndex"` // 只存哈希
	CreatedAtMs  int64  `gorm:"autoCreateTime:milli"`
	LastUsedAtMs *int64 `gorm:"index"`
	RevokedAtMs  *int64 `gorm:"index"`
}

func (m *APIKey) TableName() string {
	return model.PowerXSchema + "." + model.TableIAMAPIKey
}

func (m *APIKey) GetTableName(needFull bool) string {
	if needFull {
		return m.TableName()
	}
	return model.TableIAMAPIKey
}
