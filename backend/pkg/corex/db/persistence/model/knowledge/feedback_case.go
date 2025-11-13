package knowledge

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

// FeedbackCase 追踪反馈、再加工与 SLA。
type FeedbackCase struct {
	coremodel.PowerUUIDModel

	SpaceUUID       uuid.UUID      `gorm:"column:space_uuid;type:uuid;not null;index" json:"space_uuid"`
	ReportedBy      string         `gorm:"column:reported_by;type:varchar(128);not null" json:"reported_by"`
	Severity        string         `gorm:"column:severity;type:varchar(16);not null;default:'medium';index" json:"severity"`
	Status          string         `gorm:"column:status;type:varchar(32);not null;default:'open';index" json:"status"`
	LinkedChunks    datatypes.JSON `gorm:"column:linked_chunks;type:jsonb;default:'[]'" json:"linked_chunks"`
	ToolTraceRef    string         `gorm:"column:tool_trace_ref;type:text" json:"tool_trace_ref,omitempty"`
	SLADueAt        *time.Time     `gorm:"column:sla_due_at" json:"sla_due_at,omitempty"`
	ResolutionNotes string         `gorm:"column:resolution_notes;type:text" json:"resolution_notes,omitempty"`
	ReprocessJobID  *uint64        `gorm:"column:reprocess_job_id" json:"reprocess_job_id,omitempty"`
	EscalatedAt     *time.Time     `gorm:"column:escalated_at" json:"escalated_at,omitempty"`
	ClosedAt        *time.Time     `gorm:"column:closed_at" json:"closed_at,omitempty"`
}

func (FeedbackCase) TableName() string {
	return tableName(coremodel.TableKnowledgeFeedbackCases)
}

const (
	FeedbackSeverityLow      = "low"
	FeedbackSeverityMedium   = "medium"
	FeedbackSeverityHigh     = "high"
	FeedbackSeverityCritical = "critical"

	FeedbackStatusOpen        = "open"
	FeedbackStatusInProgress  = "in_progress"
	FeedbackStatusReprocessed = "reprocessed"
	FeedbackStatusEscalated   = "escalated"
	FeedbackStatusClosed      = "closed"
)
