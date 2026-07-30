package workflow

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

// HumanReviewTask 记录 Workflow human.review 节点创建的人工审核任务。
type HumanReviewTask struct {
	coremodel.PowerUUIDModel

	TenantUUID           string         `gorm:"column:tenant_uuid;type:varchar(128);not null;index:idx_workflow_review_tasks_tenant_status,priority:1" json:"tenant_uuid"`
	WorkflowInstanceUUID uuid.UUID      `gorm:"column:workflow_instance_uuid;type:uuid;not null;index:idx_workflow_review_tasks_instance" json:"workflow_instance_uuid"`
	StepID               string         `gorm:"column:step_id;type:varchar(128);not null;index:idx_workflow_review_tasks_step" json:"step_id"`
	ReviewType           string         `gorm:"column:review_type;type:varchar(96);not null;index:idx_workflow_review_tasks_type" json:"review_type"`
	Payload              datatypes.JSON `gorm:"column:payload;type:jsonb;not null;default:'{}'::jsonb" json:"payload,omitempty"`
	ApproverPolicy       datatypes.JSON `gorm:"column:approver_policy;type:jsonb;not null;default:'{}'::jsonb" json:"approver_policy,omitempty"`
	Status               string         `gorm:"column:status;type:varchar(32);not null;default:'pending';index:idx_workflow_review_tasks_tenant_status,priority:2" json:"status"`
	ReviewerUserUUID     uuid.UUID      `gorm:"column:reviewer_user_uuid;type:uuid;index:idx_workflow_review_tasks_reviewer" json:"reviewer_user_uuid,omitempty"`
	Decision             string         `gorm:"column:decision;type:varchar(64)" json:"decision,omitempty"`
	DecisionPayload      datatypes.JSON `gorm:"column:decision_payload;type:jsonb;not null;default:'{}'::jsonb" json:"decision_payload,omitempty"`
	Comment              string         `gorm:"column:comment;type:text" json:"comment,omitempty"`
	DueAt                *time.Time     `gorm:"column:due_at;type:timestamp with time zone;index:idx_workflow_review_tasks_due_at" json:"due_at,omitempty"`
	CompletedAt          *time.Time     `gorm:"column:completed_at;type:timestamp with time zone" json:"completed_at,omitempty"`
}

func (HumanReviewTask) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableWorkflowHumanReviewTasks
}

func (m *HumanReviewTask) GetTableName(full bool) string {
	if full {
		return m.TableName()
	}
	return coremodel.TableWorkflowHumanReviewTasks
}

func (m *HumanReviewTask) BeforeCreate(tx *gorm.DB) error {
	if err := m.PowerUUIDModel.BeforeCreate(tx); err != nil {
		return err
	}
	if strings.TrimSpace(m.TenantUUID) == "" {
		return errors.New("tenant_uuid is required")
	}
	if m.WorkflowInstanceUUID == uuid.Nil {
		return errors.New("workflow_instance_uuid is required")
	}
	if strings.TrimSpace(m.StepID) == "" {
		return errors.New("step_id is required")
	}
	if strings.TrimSpace(m.ReviewType) == "" {
		return errors.New("review_type is required")
	}
	if strings.TrimSpace(m.Status) == "" {
		m.Status = "pending"
	}
	return nil
}
