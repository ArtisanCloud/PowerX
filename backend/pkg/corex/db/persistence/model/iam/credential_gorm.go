// pkg/corex/db/persistence/model/iam/credential_gorm.go
package iam

import "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"

// 统一抽象密码/第三方认证：password/wechat/github/...
type Credential struct {
	model.PowerModel
	UserID     uint64 `gorm:"column:user_id;index;not null"                                                                              json:"user_id"`
	Provider   string `gorm:"column:provider;type:varchar(32);not null;index:uk_cred_provider_identifier,unique,priority:1"              json:"provider"`   // password|wechat|github...
	Identifier string `gorm:"column:identifier;type:varchar(128);not null;index:uk_cred_provider_identifier,unique,priority:2"           json:"identifier"` // username/email/phone/openid
	SecretHash string `gorm:"column:secret_hash;type:varchar(255)"                                                                        json:"secret_hash,omitempty"`
	IsPrimary  bool   `gorm:"column:is_primary;default:false"                                                                             json:"is_primary"`
}

func (mdl *Credential) TableName() string { return model.PowerXSchema + "." + model.TableIAMCredential }
func (mdl *Credential) GetTableName(needFull bool) string {
	if needFull {
		return mdl.TableName()
	}
	return model.TableIAMCredential
}
