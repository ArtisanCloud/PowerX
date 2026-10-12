package customer

import (
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/datatypes"
)

// AccountImport 保存确认前的输入快照及逐行执行结果；不包含登录身份或密码。
type AccountImport struct {
	coremodel.PowerUUIDModel
	TenantUUID  string         `gorm:"type:uuid;not null;index:idx_customer_import_owner,priority:1"`
	ActorUserID uint64         `gorm:"not null;index:idx_customer_import_owner,priority:2"`
	FileName    string         `gorm:"type:varchar(160);not null"`
	Checksum    string         `gorm:"type:varchar(64);not null"`
	Version     int            `gorm:"not null"`
	Status      string         `gorm:"type:varchar(24);not null"`
	Mode        string         `gorm:"type:varchar(32);not null;default:create"`
	Rows        datatypes.JSON `gorm:"type:jsonb;not null"`
	ExpiresAt   time.Time      `gorm:"not null;index"`
	TraceID     string         `gorm:"type:varchar(128)"`
}

func (AccountImport) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableCustomerAccountImports
}
func (m AccountImport) GetTableName(full bool) string {
	if full {
		return m.TableName()
	}
	return coremodel.TableCustomerAccountImports
}
