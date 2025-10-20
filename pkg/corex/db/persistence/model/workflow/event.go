package workflow

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

// WorkflowEvent 提供工作流状态变化与人工操作的审计记录。
type WorkflowEvent struct {
	coremodel.PowerModel

	TenantID          uint64         `gorm:"column:tenant_id;type:bigint;not null;index:idx_workflow_events_tenant" json:"tenant_id"`
	WorkflowUUID      uuid.UUID      `gorm:"column:instance_uuid;type:uuid;not null;index:idx_workflow_events_instance" json:"instance_uuid"`
	EventType         string         `gorm:"column:event_type;type:varchar(64);not null;index:idx_workflow_events_type" json:"event_type"`
	OccurredAt        time.Time      `gorm:"column:occurred_at;type:timestamp with time zone;not null;index:idx_workflow_events_occurred" json:"occurred_at"`
	ActorType         string         `gorm:"column:actor_type;type:varchar(32)" json:"actor_type,omitempty"`
	ActorID           string         `gorm:"column:actor_id;type:varchar(128)" json:"actor_id,omitempty"`
	Summary           string         `gorm:"column:summary;type:text" json:"summary,omitempty"`
	Payload           datatypes.JSON `gorm:"column:payload;type:jsonb;not null;default:'{}'::jsonb" json:"payload,omitempty"`
	CorrelationID     string         `gorm:"column:correlation_id;type:varchar(128);index" json:"correlation_id,omitempty"`
	RelatedStepRecord uint64         `gorm:"column:step_record_id;type:bigint" json:"step_record_id,omitempty"`
}

func (WorkflowEvent) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableWorkflowEvents
}

func (m *WorkflowEvent) GetTableName(full bool) string {
	if full {
		return m.TableName()
	}
	return coremodel.TableWorkflowEvents
}

func (m *WorkflowEvent) BeforeCreate(tx *gorm.DB) error {
	if m.OccurredAt.IsZero() {
		m.OccurredAt = time.Now().UTC()
	}
	return nil
}
