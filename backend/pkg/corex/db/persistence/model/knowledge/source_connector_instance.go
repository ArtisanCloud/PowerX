package knowledge

import (
	"strings"

	"gorm.io/datatypes"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

// SourceConnectorInstance binds a provider connector to a tenant credential (tenant-level reusable).
type SourceConnectorInstance struct {
	coremodel.PowerUUIDModel

	TenantUUID string `gorm:"column:tenant_uuid;type:varchar(128);not null;index:idx_knowledge_source_connector_tenant,priority:1" json:"tenant_uuid"`
	Provider   string `gorm:"column:provider;type:varchar(32);not null;index:idx_knowledge_source_connector_tenant,priority:2" json:"provider"`

	CredentialUUID string `gorm:"column:credential_uuid;type:uuid;not null;index" json:"credential_uuid"`

	Status    string         `gorm:"column:status;type:varchar(32);not null;default:'active';index" json:"status"`
	Config    datatypes.JSON `gorm:"column:config;type:jsonb;default:'{}'" json:"config,omitempty"`
	LastError string         `gorm:"column:last_error;type:text" json:"last_error,omitempty"`

	CreatedBy string `gorm:"column:created_by;type:varchar(128)" json:"created_by,omitempty"`
	UpdatedBy string `gorm:"column:updated_by;type:varchar(128)" json:"updated_by,omitempty"`
}

func (SourceConnectorInstance) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableKnowledgeSourceConnectorInstances
}

func (c *SourceConnectorInstance) Normalize() {
	if c == nil {
		return
	}
	c.TenantUUID = strings.ToLower(strings.TrimSpace(c.TenantUUID))
	c.Provider = strings.ToLower(strings.TrimSpace(c.Provider))
	c.Status = strings.ToLower(strings.TrimSpace(c.Status))
	c.CredentialUUID = strings.TrimSpace(c.CredentialUUID)
}
