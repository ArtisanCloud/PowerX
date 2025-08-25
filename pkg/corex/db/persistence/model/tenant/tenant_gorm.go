// pkg/corex/db/persistence/model/iam/tenant_gorm.go
package tenant

import "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"

const (
	TenantPlanFree = "free"
	TenantPlanPro  = "pro"
)

const (
	TenantStatusActive   = 1
	TenantStatusDisabled = 0
)

type Tenant struct {
	model.PowerUUIDModel

	Key         string `gorm:"column:key;type:varchar(64);uniqueIndex;not null" json:"key"` // 全局唯一，如 system、acme
	Name        string `gorm:"column:name;type:varchar(128);not null"            json:"name"`
	Status      int16  `gorm:"column:status;default:1;index"                     json:"status"` // 1=active,0=disabled
	Plan        string `gorm:"column:plan;type:varchar(64);default:'free'"       json:"plan"`
	Domain      string `gorm:"column:domain;type:varchar(256);uniqueIndex" json:"domain"`
	Description string `gorm:"column:description;type:text"                  json:"description"`
}

func (mdl *Tenant) TableName() string {
	return model.PowerXSchema + "." + model.TableIAMTenant
}
func (mdl *Tenant) GetTableName(needFull bool) string {
	if needFull {
		return mdl.TableName()
	}
	return model.TableIAMTenant
}

const SystemTenantKey = "system"

func (m *Tenant) ToLite() map[string]any {
	return map[string]any{
		"id":     m.ID,
		"uuid":   m.UUID,
		"key":    m.Key,
		"name":   m.Name,
		"status": m.Status,
		"plan":   m.Plan,
		"domain": m.Domain,
	}
}
