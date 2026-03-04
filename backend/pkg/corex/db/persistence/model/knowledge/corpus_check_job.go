package knowledge

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

// CorpusCheckJob 记录一次语料体检任务（异步），输出统计指标与推荐策略卡片。
type CorpusCheckJob struct {
	coremodel.PowerUUIDModel

	TenantUUID string    `gorm:"column:tenant_uuid;type:varchar(128);not null;index:idx_corpus_check_tenant_space,priority:1" json:"tenant_uuid"`
	SpaceUUID  uuid.UUID `gorm:"column:space_uuid;type:uuid;not null;index:idx_corpus_check_tenant_space,priority:2;index" json:"space_uuid"`
	Status     string    `gorm:"column:status;type:varchar(32);not null;default:'pending';index" json:"status"`

	SampleJobUUIDs datatypes.JSON `gorm:"column:sample_job_uuids;type:jsonb;not null;default:'[]'" json:"sample_job_uuids"`
	Metrics        datatypes.JSON `gorm:"column:metrics;type:jsonb;not null;default:'{}'" json:"metrics"`
	Recommendations datatypes.JSON `gorm:"column:recommendations;type:jsonb;not null;default:'[]'" json:"recommendations"`

	TraceID     string     `gorm:"column:trace_id;type:varchar(128);index" json:"trace_id,omitempty"`
	ErrorReason string     `gorm:"column:error_reason;type:text" json:"error_reason,omitempty"`
	StartedAt   *time.Time `gorm:"column:started_at" json:"started_at,omitempty"`
	CompletedAt *time.Time `gorm:"column:completed_at" json:"completed_at,omitempty"`
}

func (CorpusCheckJob) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableKnowledgeCorpusCheckJobs
}

const (
	CorpusCheckStatusPending   = "pending"
	CorpusCheckStatusRunning   = "running"
	CorpusCheckStatusCompleted = "completed"
	CorpusCheckStatusFailed    = "failed"
)

