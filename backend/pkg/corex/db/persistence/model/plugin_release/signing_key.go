package plugin_release

import (
	"strings"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

// SigningKey is Core-owned trust material for offline plugin packages. The
// public key is never accepted from a package-registration request.
type SigningKey struct {
	coremodel.PowerUUIDModel
	KeyID     string `gorm:"column:key_id;type:varchar(128);not null;uniqueIndex" json:"key_id"`
	PublicKey string `gorm:"column:public_key;type:text;not null" json:"-"`
	Enabled   bool   `gorm:"column:enabled;not null;default:true;index" json:"enabled"`
}

func (SigningKey) TableName() string {
	schema := strings.TrimSpace(coremodel.PowerXSchema)
	if schema == "" {
		return "plugin_release_signing_keys"
	}
	return schema + ".plugin_release_signing_keys"
}
