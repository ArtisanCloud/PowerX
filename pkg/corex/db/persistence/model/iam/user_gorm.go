// pkg/corex/db/persistence/model/iam/user_gorm.go
package iam

import (
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/datatypes"
)

type User struct {
	model.PowerUUIDModel

	TenantID    uint64         `gorm:"column:tenant_id;index;not null"                                                                 json:"tenant_id"`
	Username    string         `gorm:"column:username;type:varchar(64);index:uk_user_tenant_username,unique,priority:2"                json:"username"`
	Email       string         `gorm:"column:email;type:varchar(128);index"                                                            json:"email,omitempty"`
	Phone       string         `gorm:"column:phone;type:varchar(32);index"                                                             json:"phone,omitempty"`
	DisplayName string         `gorm:"column:display_name;type:varchar(128)"                                                           json:"display_name,omitempty"`
	AvatarURL   string         `gorm:"column:avatar_url;type:varchar(512)"                                                             json:"avatar_url,omitempty"`
	Status      int16          `gorm:"column:status;default:1;index"                                                                   json:"status"`
	Meta        datatypes.JSON `gorm:"column:meta"                                                                                     json:"meta,omitempty"`

	// 复合唯一：tenant_id + username
	// 通过 priority:1/2 与 username 的索引共同实现
	_ struct{} `gorm:"index:uk_user_tenant_username,unique,priority:1,expression:tenant_id"` // 装饰位（仅为表意；如不需要可移除）

	LastLoginAt *int64 `gorm:"column:last_login_at" json:"last_login_at,omitempty"`
}

func (mdl *User) TableName() string {
	return model.PowerXSchema + "." + model.TableIAMUser
}
func (mdl *User) GetTableName(needFull bool) string {
	if needFull {
		return mdl.TableName()
	}
	return model.TableIAMUser
}
