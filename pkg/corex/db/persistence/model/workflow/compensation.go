package workflow

import (
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

// WorkflowStepCompensation 记录补偿步骤的执行状态。
type WorkflowStepCompensation struct {
	coremodel.PowerModel

	StepRecordID uint64     `gorm:"column:step_record_id;type:bigint;not null;index:idx_workflow_compensations_step" json:"step_record_id"`
	State        string     `gorm:"column:state;type:varchar(32);not null;default:'pending';index:idx_workflow_compensations_state" json:"state"`
	Handler      string     `gorm:"column:handler;type:varchar(128);not null" json:"handler"`
	InitiatedBy  string     `gorm:"column:initiated_by;type:varchar(32);not null;default:'auto'" json:"initiated_by"`
	Notes        string     `gorm:"column:notes;type:text" json:"notes,omitempty"`
	StartedAt    *time.Time `gorm:"column:started_at;type:timestamp with time zone" json:"started_at,omitempty"`
	CompletedAt  *time.Time `gorm:"column:completed_at;type:timestamp with time zone" json:"completed_at,omitempty"`
}

func (WorkflowStepCompensation) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableWorkflowStepCompensations
}

func (m *WorkflowStepCompensation) GetTableName(full bool) string {
	if full {
		return m.TableName()
	}
	return coremodel.TableWorkflowStepCompensations
}
