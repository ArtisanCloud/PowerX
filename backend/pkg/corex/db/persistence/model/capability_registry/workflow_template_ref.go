package capability_registry

import (
	"time"

	"gorm.io/datatypes"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

// WorkflowTemplateRef 存储插件随能力提供的 Workflow/Composite 模板。
type WorkflowTemplateRef struct {
	coremodel.PowerModel

	CapabilityID string         `gorm:"column:capability_id;type:varchar(128);not null;index:idx_workflow_template_capability" json:"capability_id"`
	TemplateID   string         `gorm:"column:template_id;type:varchar(128);not null;uniqueIndex:uk_capability_template" json:"template_id"`
	Name         string         `gorm:"column:name;type:varchar(128);not null" json:"name"`
	Description  string         `gorm:"column:description;type:text" json:"description,omitempty"`
	Steps        datatypes.JSON `gorm:"column:steps;type:jsonb;default:'[]'" json:"steps,omitempty"`
	ParamsSchema datatypes.JSON `gorm:"column:params_schema;type:jsonb;default:'{}'" json:"params_schema,omitempty"`

	ProtocolRequirements  datatypes.JSON `gorm:"column:protocol_requirements;type:jsonb;default:'[]'" json:"protocol_requirements,omitempty"`
	CapabilitiesHash      string         `gorm:"column:capabilities_hash_snapshot;type:varchar(128);not null;index:idx_workflow_template_hash" json:"capabilities_hash_snapshot"`
	TemplateHash          string         `gorm:"column:template_hash;type:varchar(128);not null" json:"template_hash"`
	RequiresManualUpgrade bool           `gorm:"column:requires_manual_upgrade;not null;default:true" json:"requires_manual_upgrade"`
	LastSyncedAt          *time.Time     `gorm:"column:last_synced_at" json:"last_synced_at,omitempty"`
}

func (WorkflowTemplateRef) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableCapabilityRegistryWorkflowTemplateRef
}
