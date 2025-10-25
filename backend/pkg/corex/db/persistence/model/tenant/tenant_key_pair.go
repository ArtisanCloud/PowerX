package tenant

import (
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/datatypes"
)

type TenantKeyPair struct {
	model.PowerModel

	// 作用域（建议直接写字段，便于在本实体上声明索引）
	model.ScopeRef

	// 密钥标识与算法
	KID string `gorm:"size:64;index" json:"kid"` // 如 "t:<tid>:v1"
	Alg string `gorm:"size:32"       json:"alg"` // "RSA-OAEP-256"

	// 公钥明文（PEM）
	PublicPEM string `gorm:"type:text" json:"-"`

	// 私钥密文（JSON 封装：AES-GCM 包裹）
	// 例如：{"wrapAlg":"AES-GCM","nonce":"...","ct":"...","ts":123456}
	EncPrivate datatypes.JSONMap `gorm:"type:jsonb;default:'{}'::jsonb" json:"-"`

	// 是否当前激活
	Active bool `gorm:"default:true" json:"-"`
}

func (mdl *TenantKeyPair) TableName() string {
	return model.PowerXSchema + "." + model.TableTenantKeyPair
}
func (mdl *TenantKeyPair) GetTableName(needFull bool) string {
	if needFull {
		return mdl.TableName()
	}
	return model.TableTenantKeyPair
}
