package knowledge

import (
	"gorm.io/datatypes"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

// IndexJob records the accepted asynchronous action. It is deliberately
// separate from the legacy ingestion job so every Host Contract operation has
// a stable operation, tenant and public UUID.
type IndexJob struct {
	coremodel.PowerUUIDModel

	TenantUUID       string         `gorm:"column:tenant_uuid;type:varchar(128);not null;index" json:"tenant_uuid"`
	SpaceUUID        string         `gorm:"column:space_uuid;type:uuid;not null;index" json:"space_uuid"`
	DocumentUUID     *string        `gorm:"column:document_uuid;type:uuid;index" json:"document_uuid,omitempty"`
	Operation        string         `gorm:"column:operation;type:varchar(32);not null;index" json:"operation"`
	Status           string         `gorm:"column:status;type:varchar(32);not null;default:'queued';index" json:"status"`
	Priority         string         `gorm:"column:priority;type:varchar(16);not null;default:'normal';index:idx_knowledge_host_job_queue,priority:2" json:"priority"`
	RequestedConfig  datatypes.JSON `gorm:"column:requested_config;type:jsonb" json:"-"`
	ConfigSnapshot   datatypes.JSON `gorm:"column:config_snapshot;type:jsonb" json:"-"`
	SnapshotChecksum string         `gorm:"column:snapshot_checksum;type:char(64)" json:"snapshot_checksum"`
	SourceSnapshot   datatypes.JSON `gorm:"column:source_snapshot;type:jsonb" json:"-"`
	TraceID          string         `gorm:"column:trace_id;type:varchar(64)" json:"trace_id"`
	ClaimToken       string         `gorm:"column:claim_token;type:varchar(64)" json:"-"`
	LeaseUntil       *time.Time     `gorm:"column:lease_until;index" json:"-"`
	ChunkCount       int            `gorm:"column:chunk_count;not null;default:0" json:"chunk_count"`
	ErrorCode        string         `gorm:"column:error_code;type:varchar(64)" json:"error_code,omitempty"`
	StartedAt        *time.Time     `gorm:"column:started_at" json:"started_at,omitempty"`
	CompletedAt      *time.Time     `gorm:"column:completed_at" json:"completed_at,omitempty"`
}

func (IndexJob) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableKnowledgeIndexJobs
}
