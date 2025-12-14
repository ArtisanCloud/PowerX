// pkg/corex/db/persistence/model/iam/position_gorm.go
package iam

import "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"

type Position struct {
	ID         uint64 `gorm:"primaryKey"`
	TenantUUID string `gorm:"column:tenant_uuid;type:char(36);index;not null"`
	Key        string `gorm:"type:varchar(64);not null;index:,unique,composite:uk_pos_tenant_key"`
	Name       string `gorm:"type:varchar(128);not null"`
	Level      int16  `gorm:"index"` // 职级
	Status     int16  `gorm:"index;default:1"`
}

func (Position) TableName() string { return model.PowerXSchema + "." + model.TableIAMPosition }

func (Position) GetTableName(needFull bool) string {
	if needFull {
		return Position{}.TableName()
	}
	return model.TableIAMPosition
}
