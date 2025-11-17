package knowledge

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

// IngestionJob 记录每次入库任务的状态与指标。
type IngestionJob struct {
	coremodel.PowerUUIDModel

	SpaceUUID           uuid.UUID      `gorm:"column:space_uuid;type:uuid;not null;index" json:"space_uuid"`
	SourceID            string         `gorm:"column:source_id;type:varchar(128);not null" json:"source_id"`
	SourceType          string         `gorm:"column:source_type;type:varchar(32);not null" json:"source_type"`
	Status              string         `gorm:"column:status;type:varchar(32);not null;default:'pending';index" json:"status"`
	Priority            string         `gorm:"column:priority;type:varchar(16);not null;default:'normal'" json:"priority"`
	RetryCount          int            `gorm:"column:retry_count;type:int;not null;default:0" json:"retry_count"`
	ChunkTotal          int            `gorm:"column:chunk_total;type:int;not null;default:0" json:"chunk_total"`
	ChunkCoveredPct     float64        `gorm:"column:chunk_covered_pct;type:numeric(5,2);not null;default:0" json:"chunk_covered_pct"`
	SummaryChunkCount   int            `gorm:"column:summary_chunk_count;type:int;not null;default:0" json:"summary_chunk_count"`
	ParagraphChunkCount int            `gorm:"column:paragraph_chunk_count;type:int;not null;default:0" json:"paragraph_chunk_count"`
	EmbeddingSuccessPct float64        `gorm:"column:embedding_success_pct;type:numeric(5,2);not null;default:0" json:"embedding_success_pct"`
	MaskingCoveragePct  float64        `gorm:"column:masking_coverage_pct;type:numeric(5,2);not null;default:0" json:"masking_coverage_pct"`
	ErrorCode           string         `gorm:"column:error_code;type:varchar(64)" json:"error_code,omitempty"`
	BlockedReason       string         `gorm:"column:blocked_reason;type:text" json:"blocked_reason,omitempty"`
	SubmittedBy         string         `gorm:"column:submitted_by;type:varchar(128)" json:"submitted_by,omitempty"`
	StartedAt           *time.Time     `gorm:"column:started_at" json:"started_at,omitempty"`
	CompletedAt         *time.Time     `gorm:"column:completed_at" json:"completed_at,omitempty"`
	ArtifactBundleID    *uint64        `gorm:"column:artifact_bundle_id" json:"artifact_bundle_id,omitempty"`
	MetricsSnapshot     datatypes.JSON `gorm:"column:metrics_snapshot;type:jsonb;default:'{}'" json:"metrics_snapshot"`
}

func (IngestionJob) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableKnowledgeIngestionJobs
}

const (
	IngestionStatusPending   = "pending"
	IngestionStatusRunning   = "running"
	IngestionStatusRetrying  = "retrying"
	IngestionStatusPaused    = "paused"
	IngestionStatusCompleted = "completed"
	IngestionStatusFailed    = "failed"
	IngestionStatusBlocked   = "blocked"
)
