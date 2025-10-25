package workflow

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

// AgentAssignment 记录工作流步骤与 Agent 的派发关系。
type AgentAssignment struct {
	coremodel.PowerModel

	StepRecordID   uint64     `gorm:"column:step_record_id;type:bigint;not null;index:idx_workflow_assignments_step" json:"step_record_id"`
	AgentUUID      uuid.UUID  `gorm:"column:agent_uuid;type:uuid;not null;index:idx_workflow_assignments_agent" json:"agent_uuid"`
	Status         string     `gorm:"column:status;type:varchar(32);not null;default:'dispatched';index:idx_workflow_assignments_status" json:"status"`
	DispatchedAt   time.Time  `gorm:"column:dispatched_at;type:timestamp with time zone;not null" json:"dispatched_at"`
	AcknowledgedAt *time.Time `gorm:"column:acknowledged_at;type:timestamp with time zone" json:"acknowledged_at,omitempty"`
	AckDeadline    *time.Time `gorm:"column:ack_deadline;type:timestamp with time zone" json:"ack_deadline,omitempty"`
	CompletedAt    *time.Time `gorm:"column:completed_at;type:timestamp with time zone" json:"completed_at,omitempty"`
	LastHeartbeat  *time.Time `gorm:"column:last_heartbeat_at;type:timestamp with time zone" json:"last_heartbeat_at,omitempty"`
}

func (AgentAssignment) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableWorkflowAgentAssignments
}

func (m *AgentAssignment) GetTableName(full bool) string {
	if full {
		return m.TableName()
	}
	return coremodel.TableWorkflowAgentAssignments
}

func (m *AgentAssignment) BeforeCreate(tx *gorm.DB) error {
	if m.DispatchedAt.IsZero() {
		m.DispatchedAt = time.Now().UTC()
	}
	return nil
}
