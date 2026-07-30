package workflow

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

// WorkflowStepRecord 记录单个步骤的执行状态与输入输出。
type WorkflowStepRecord struct {
	coremodel.PowerModel

	InstanceUUID   uuid.UUID      `gorm:"column:instance_uuid;type:uuid;not null;index:idx_workflow_step_records_instance" json:"instance_uuid"`
	StepID         string         `gorm:"column:step_id;type:varchar(128);not null;index:idx_workflow_step_records_step" json:"step_id"`
	Type           string         `gorm:"column:type;type:varchar(32);not null" json:"type"`
	NodeKind       string         `gorm:"column:node_kind;type:varchar(96);index:idx_workflow_step_records_node_kind" json:"node_kind,omitempty"`
	NodeRef        string         `gorm:"column:node_ref;type:varchar(256);index:idx_workflow_step_records_node_ref" json:"node_ref,omitempty"`
	State          string         `gorm:"column:state;type:varchar(32);not null;default:'queued';index:idx_workflow_step_records_state" json:"state"`
	SubjectType    string         `gorm:"column:subject_type;type:varchar(32);not null" json:"subject_type"`
	SubjectUUID    uuid.UUID      `gorm:"column:subject_uuid;type:uuid" json:"subject_uuid"`
	ToolGrantID    string         `gorm:"column:tool_grant_id;type:varchar(128)" json:"tool_grant_id,omitempty"`
	ToolGrantVer   int64          `gorm:"column:tool_grant_version;type:bigint" json:"tool_grant_version,omitempty"`
	Attempt        int32          `gorm:"column:attempt;type:int;not null;default:0" json:"attempt"`
	InputMapping   datatypes.JSON `gorm:"column:input_mapping;type:jsonb;not null;default:'{}'::jsonb" json:"input_mapping,omitempty"`
	OutputMapping  datatypes.JSON `gorm:"column:output_mapping;type:jsonb;not null;default:'{}'::jsonb" json:"output_mapping,omitempty"`
	PayloadIn      datatypes.JSON `gorm:"column:payload_in;type:jsonb;not null;default:'{}'::jsonb" json:"payload_in,omitempty"`
	PayloadOut     datatypes.JSON `gorm:"column:payload_out;type:jsonb;not null;default:'{}'::jsonb" json:"payload_out,omitempty"`
	FailureReason  string         `gorm:"column:failure_reason;type:text" json:"failure_reason,omitempty"`
	ErrorCode      string         `gorm:"column:error_code;type:varchar(128);index:idx_workflow_step_records_error_code" json:"error_code,omitempty"`
	ErrorMessage   string         `gorm:"column:error_message;type:text" json:"error_message,omitempty"`
	ScheduledAt    time.Time      `gorm:"column:scheduled_at;type:timestamp with time zone;index:idx_workflow_step_records_scheduled" json:"scheduled_at"`
	LeaseOwner     string         `gorm:"column:lease_owner;type:varchar(128);index:idx_workflow_step_records_lease_owner" json:"lease_owner,omitempty"`
	LeaseUntil     *time.Time     `gorm:"column:lease_until;type:timestamp with time zone;index:idx_workflow_step_records_lease_until" json:"lease_until,omitempty"`
	StartedAt      *time.Time     `gorm:"column:started_at;type:timestamp with time zone" json:"started_at,omitempty"`
	CompletedAt    *time.Time     `gorm:"column:completed_at;type:timestamp with time zone" json:"completed_at,omitempty"`
	LastTransition time.Time      `gorm:"column:last_transition_at;type:timestamp with time zone;index:idx_workflow_step_records_transition" json:"last_transition_at"`
	AwaitingHuman  bool           `gorm:"column:awaiting_human;not null;default:false" json:"awaiting_human"`
}

func (WorkflowStepRecord) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableWorkflowStepRecords
}

func (m *WorkflowStepRecord) GetTableName(full bool) string {
	if full {
		return m.TableName()
	}
	return coremodel.TableWorkflowStepRecords
}

func (m *WorkflowStepRecord) BeforeCreate(tx *gorm.DB) error {
	if m.InstanceUUID == uuid.Nil {
		return errors.New("instance_uuid is required")
	}
	if m.ScheduledAt.IsZero() {
		m.ScheduledAt = time.Now().UTC()
	}
	if m.LastTransition.IsZero() {
		m.LastTransition = m.ScheduledAt
	}
	if m.State == "" {
		m.State = "queued"
	}
	return nil
}
