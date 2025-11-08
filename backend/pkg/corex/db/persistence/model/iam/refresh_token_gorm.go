// pkg/corex/db/persistence/model/iam/refresh_token_gorm.go
package iam

import "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"

type RefreshToken struct {
	model.PowerModel
	JTI        string `gorm:"column:jti;type:varchar(64);uniqueIndex;not null" json:"jti"`
	UserUUID   string `gorm:"column:user_uuid;index;not null"                    json:"user_uuid"`
	MemberUUID string `gorm:"column:member_uuid;index;not null"                    json:"member_uuid"`
	TenantUUID string `gorm:"column:tenant_uuid;index;not null"                  json:"tenant_uuid"`
	ExpiresAt  int64  `gorm:"column:expires_at;index;not null"                 json:"expires_at"` // Unix milli
	RevokedAt  *int64 `gorm:"column:revoked_at;index"                          json:"revoked_at,omitempty"`
	IP         string `gorm:"column:ip;type:varchar(45)"                        json:"ip,omitempty"`
	UserAgent  string `gorm:"column:user_agent;type:varchar(256)"               json:"user_agent,omitempty"`
}

func (mdl *RefreshToken) TableName() string {
	return model.PowerXSchema + "." + model.TableIAMRefreshToken
}
func (mdl *RefreshToken) GetTableName(needFull bool) string {
	if needFull {
		return mdl.TableName()
	}
	return model.TableIAMRefreshToken
}
