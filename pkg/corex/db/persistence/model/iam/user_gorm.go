// pkg/corex/db/persistence/model/iam/user_gorm.go
package iam

import (
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/datatypes"
)

// pkg/corex/db/persistence/model/iam/user_gorm.go
type User struct {
	model.PowerUUIDModel

	Email       string         `gorm:"column:email;type:varchar(128);index"                                           json:"email,omitempty"`
	Phone       string         `gorm:"column:phone;type:varchar(32);index"                                            json:"phone,omitempty"`
	DisplayName string         `gorm:"column:display_name;type:varchar(128)"                                          json:"display_name,omitempty"`
	AvatarURL   string         `gorm:"column:avatar_url;type:varchar(512)"                                            json:"avatar_url,omitempty"`
	Status      int16          `gorm:"column:status;default:1;index"                                                  json:"status"`
	Meta        datatypes.JSON `gorm:"column:meta;type:jsonb"                                                                     json:"meta,omitempty"`

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
