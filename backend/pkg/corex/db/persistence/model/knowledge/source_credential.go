package knowledge

import (
	"strings"

	"gorm.io/datatypes"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

// SourceCredential stores tenant-scoped auth material references for external knowledge sources (Notion/Feishu/etc).
// NOTE: The secret itself SHOULD be stored in a secret store; this model keeps only references and metadata.
type SourceCredential struct {
	coremodel.PowerUUIDModel

	TenantUUID string `gorm:"column:tenant_uuid;type:varchar(128);not null;index:idx_knowledge_source_cred_tenant,priority:1" json:"tenant_uuid"`
	Provider   string `gorm:"column:provider;type:varchar(32);not null;index:idx_knowledge_source_cred_tenant,priority:2" json:"provider"`
	AuthType   string `gorm:"column:auth_type;type:varchar(32);not null" json:"auth_type"` // oauth|token
	Label      string `gorm:"column:label;type:varchar(128);not null" json:"label"`
	Status     string `gorm:"column:status;type:varchar(32);not null;default:'active';index" json:"status"`

	SecretRef string         `gorm:"column:secret_ref;type:varchar(256)" json:"secret_ref,omitempty"`
	Metadata  datatypes.JSON `gorm:"column:metadata;type:jsonb;default:'{}'" json:"metadata,omitempty"`

	CreatedBy string `gorm:"column:created_by;type:varchar(128)" json:"created_by,omitempty"`
	UpdatedBy string `gorm:"column:updated_by;type:varchar(128)" json:"updated_by,omitempty"`
}

func (SourceCredential) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableKnowledgeSourceCredentials
}

func (c *SourceCredential) Normalize() {
	if c == nil {
		return
	}
	c.TenantUUID = strings.ToLower(strings.TrimSpace(c.TenantUUID))
	c.Provider = strings.ToLower(strings.TrimSpace(c.Provider))
	c.AuthType = strings.ToLower(strings.TrimSpace(c.AuthType))
	c.Label = strings.TrimSpace(c.Label)
	c.Status = strings.ToLower(strings.TrimSpace(c.Status))
}
