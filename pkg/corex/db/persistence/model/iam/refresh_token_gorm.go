// pkg/corex/db/persistence/model/iam/refresh_token_gorm.go
package iam

import "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"

type RefreshToken struct {
	model.PowerModel
	JTI       string `gorm:"column:jti;type:varchar(64);uniqueIndex;not null" json:"jti"`
	UserID    uint64 `gorm:"column:user_id;index;not null"                    json:"user_id"`
	TenantID  uint64 `gorm:"column:tenant_id;index;not null"                  json:"tenant_id"`
	ExpiresAt int64  `gorm:"column:expires_at;index;not null"                 json:"expires_at"` // Unix milli
	RevokedAt *int64 `gorm:"column:revoked_at;index"                          json:"revoked_at,omitempty"`
	IP        string `gorm:"column:ip;type:varchar(45)"                        json:"ip,omitempty"`
	UserAgent string `gorm:"column:user_agent;type:varchar(256)"               json:"user_agent,omitempty"`
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
