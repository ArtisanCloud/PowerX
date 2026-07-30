package workflow

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

// WorkflowInstance 记录运行时实例的状态与上下文。
type WorkflowInstance struct {
	coremodel.PowerUUIDModel

	TenantUUID        string         `gorm:"column:tenant_uuid;type:varchar(128);not null;index:idx_workflow_instances_tenant_uuid;index:idx_workflow_instances_correlation,priority:1" json:"tenant_uuid"`
	DefinitionUUID    uuid.UUID      `gorm:"column:definition_uuid;type:uuid;not null;index:idx_workflow_instances_definition" json:"definition_uuid"`
	DefinitionVersion int32          `gorm:"column:definition_version;type:int;not null;default:1" json:"definition_version"`
	State             string         `gorm:"column:state;type:varchar(32);not null;default:'draft';index:idx_workflow_instances_state" json:"state"`
	InputContext      datatypes.JSON `gorm:"column:input_context;type:jsonb;not null;default:'{}'::jsonb" json:"input_context,omitempty"`
	RuntimeContext    datatypes.JSON `gorm:"column:runtime_context;type:jsonb;not null;default:'{}'::jsonb" json:"runtime_context,omitempty"`
	OutputContext     datatypes.JSON `gorm:"column:output_context;type:jsonb;not null;default:'{}'::jsonb" json:"output_context,omitempty"`
	SlaSnapshot       datatypes.JSON `gorm:"column:sla_snapshot;type:jsonb;not null;default:'{}'::jsonb" json:"sla_snapshot,omitempty"`
	LastError         string         `gorm:"column:last_error;type:text" json:"last_error,omitempty"`
	CorrelationID     string         `gorm:"column:correlation_id;type:varchar(128);index:idx_workflow_instances_correlation,priority:2" json:"correlation_id,omitempty"`
	AgentUUID         uuid.UUID      `gorm:"column:agent_uuid;type:uuid;index:idx_workflow_instances_agent_uuid" json:"agent_uuid,omitempty"`
	InitiatorUserUUID uuid.UUID      `gorm:"column:initiator_user_uuid;type:uuid;index:idx_workflow_instances_initiator_user_uuid" json:"initiator_user_uuid,omitempty"`
	TraceID           string         `gorm:"column:trace_id;type:varchar(128);index:idx_workflow_instances_trace_id" json:"trace_id,omitempty"`
	Tags              datatypes.JSON `gorm:"column:tags;type:jsonb;not null;default:'{}'::jsonb" json:"tags,omitempty"`
	SlaDeadline       *time.Time     `gorm:"column:sla_deadline;type:timestamp with time zone;index:idx_workflow_instances_sla_deadline" json:"sla_deadline,omitempty"`
	StartedAt         *time.Time     `gorm:"column:started_at;type:timestamp with time zone;index:idx_workflow_instances_started_at" json:"started_at,omitempty"`
	CompletedAt       *time.Time     `gorm:"column:completed_at;type:timestamp with time zone;index:idx_workflow_instances_completed_at" json:"completed_at,omitempty"`

	// 汇总字段，方便查询调度器与导出逻辑。
	CurrentStepID    string    `gorm:"column:current_step_id;type:varchar(128);index:idx_workflow_instances_current_step" json:"current_step_id,omitempty"`
	LastTransitionAt time.Time `gorm:"column:last_transition_at;type:timestamp with time zone;not null;default:CURRENT_TIMESTAMP" json:"last_transition_at"`
	NextHeartbeatDue time.Time `gorm:"column:next_heartbeat_due;type:timestamp with time zone;not null;default:CURRENT_TIMESTAMP" json:"next_heartbeat_due"`
}

func (WorkflowInstance) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableWorkflowInstances
}

func (m *WorkflowInstance) GetTableName(full bool) string {
	if full {
		return m.TableName()
	}
	return coremodel.TableWorkflowInstances
}

func (m *WorkflowInstance) BeforeCreate(tx *gorm.DB) error {
	if err := m.PowerUUIDModel.BeforeCreate(tx); err != nil {
		return err
	}
	if m.DefinitionUUID == uuid.Nil {
		return errors.New("definition uuid is required")
	}
	if m.State == "" {
		m.State = "draft"
	}
	now := time.Now().UTC()
	if m.LastTransitionAt.IsZero() {
		m.LastTransitionAt = now
	}
	if m.NextHeartbeatDue.IsZero() {
		m.NextHeartbeatDue = now
	}
	return nil
}
