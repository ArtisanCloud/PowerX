package iam

import (
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/datatypes"
)

type Member struct {
	model.PowerUUIDModel

	TenantID    uint64         `gorm:"column:tenant_id;not null;index;uniqueIndex:uk_member_tenant_username"                json:"tenant_id"`
	UserID      uint64         `gorm:"column:user_id;not null;index;uniqueIndex:uk_member_tenant_user"                      json:"user_id"`
	Username    string         `gorm:"column:username;type:varchar(64);not null;uniqueIndex:uk_member_tenant_username"      json:"username"` // 建议统一小写
	DisplayName string         `gorm:"column:display_name;type:varchar(128)" json:"display_name,omitempty"`
	AvatarURL   string         `gorm:"column:avatar_url;type:varchar(512)"  json:"avatar_url,omitempty"`
	Status      int16          `gorm:"column:status;default:1;index"                                                        json:"status"`
	Meta        datatypes.JSON `gorm:"column:meta;type:jsonb"                                                                   json:"meta,omitempty"`
}

func (mdl *Member) TableName() string { return model.PowerXSchema + "." + model.TableIAMMember }

func (mdl *Member) GetTableName(needFull bool) string {
	if needFull {
		return mdl.TableName()
	}
	return model.TableIAMMember
}
