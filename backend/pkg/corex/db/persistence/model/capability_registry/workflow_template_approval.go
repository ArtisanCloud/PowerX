package capability_registry

import (
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

// WorkflowTemplateApproval 记录管理员确认的模板版本。
type WorkflowTemplateApproval struct {
	coremodel.PowerModel

	TemplateID       string    `gorm:"column:template_id;type:varchar(128);not null;uniqueIndex:uk_workflow_template_approval" json:"template_id"`
	CapabilityID     string    `gorm:"column:capability_id;type:varchar(128);not null" json:"capability_id"`
	CapabilitiesHash string    `gorm:"column:capabilities_hash;type:varchar(128);not null" json:"capabilities_hash"`
	Reason           string    `gorm:"column:reason;type:text" json:"reason,omitempty"`
	ApprovedBy       string    `gorm:"column:approved_by;type:varchar(128)" json:"approved_by,omitempty"`
	ApprovedAt       time.Time `gorm:"column:approved_at;not null" json:"approved_at"`
}

func (WorkflowTemplateApproval) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableCapabilityRegistryWorkflowTemplateApproval
}
