// pkg/corex/db/persistence/model/iam/group_gorm.go
package iam

import "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"

type Group struct {
	ID         uint64 `gorm:"primaryKey"`
	TenantUUID string `gorm:"column:tenant_uuid;type:char(36);index;not null"`
	Key        string `gorm:"type:varchar(64);not null;index:,unique,composite:uk_group_tenant_key"`
	Name       string `gorm:"type:varchar(128);not null"`
	Status     int16  `gorm:"index;default:1"`
}

func (m *Group) TableName() string { return model.PowerXSchema + "." + model.TableIAMGroup }

func (m *Group) GetTableName(needFull bool) string {
	if needFull {
		return m.TableName()
	}
	return model.TableIAMGroup
}
