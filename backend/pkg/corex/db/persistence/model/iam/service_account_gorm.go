// pkg/corex/db/persistence/model/iam/service_account_gorm.go
package iam

import "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"

type ServiceAccount struct {
	ID         uint64 `gorm:"primaryKey"`
	TenantUUID string `gorm:"column:tenant_uuid;type:char(36);index;not null"`
	Key        string `gorm:"type:varchar(64);not null;index:,unique,composite:uk_svc_tenant_key"`
	Name       string `gorm:"type:varchar(128);not null"`
	Status     int16  `gorm:"index;default:1"`
}

func (m *ServiceAccount) TableName() string {
	return model.PowerXSchema + "." + model.TableIAMServiceAccount
}

func (m *ServiceAccount) GetTableName(needFull bool) string {
	if needFull {
		return m.TableName()
	}
	return model.TableIAMServiceAccount
}

// 也可用 Credential 体系做密钥（沿用 provider/identifier）:contentReference[oaicite:15]{index=15}
