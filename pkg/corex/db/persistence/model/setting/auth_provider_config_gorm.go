// pkg/corex/db/persistence/model/setting/auth_provider_config.go
package setting

import (
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/datatypes"
)

// 身份提供商（IdP）非密配置；密钥类放密管/ENV，仅在此保存“引用名”
type AuthProviderConfig struct {
	coremodel.PowerModel

	TenantID uint64 `gorm:"column:tenant_id;not null;index:idx_authp_tenant_type,priority:1" json:"tenant_id"`

	Type    string         `gorm:"column:type;type:varchar(32);not null;index:idx_authp_tenant_type,priority:2;uniqueIndex:uk_authp_tenant_type,priority:1" json:"type"` // builtin|oidc|saml|ldap|wechat
	Config  datatypes.JSON `gorm:"column:config_json;type:jsonb" json:"config_json,omitempty"`                                                                           // 非密参：issuer、scopes、映射等
	Enabled bool           `gorm:"column:enabled;default:false;index" json:"enabled"`

	// 可选：回调 URLs 的校验状态/最后一次健康检查
	Verified   bool    `gorm:"column:verified;default:false;index" json:"verified"`
	VerifyNote *string `gorm:"column:verify_note;type:varchar(255)" json:"verify_note,omitempty"`
}

func (m *AuthProviderConfig) TableName() string {
	return coremodel.PowerXSchema + "." + TableAuthProviderConfig
}
func (m *AuthProviderConfig) GetTableName(full bool) string {
	if full {
		return m.TableName()
	}
	return TableAuthProviderConfig
}
