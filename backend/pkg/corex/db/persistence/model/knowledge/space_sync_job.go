package knowledge

import (
	"strings"
	"time"

	"gorm.io/datatypes"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

// SpaceSyncJob represents a space-scoped incremental sync job bound to a tenant connector instance.
type SpaceSyncJob struct {
	coremodel.PowerUUIDModel

	TenantUUID string `gorm:"column:tenant_uuid;type:varchar(128);not null;index:idx_knowledge_space_sync_job_scope,priority:1" json:"tenant_uuid"`
	SpaceUUID  string `gorm:"column:space_uuid;type:uuid;not null;index:idx_knowledge_space_sync_job_scope,priority:2" json:"space_uuid"`

	ConnectorUUID string `gorm:"column:connector_uuid;type:uuid;not null;index" json:"connector_uuid"`
	Provider      string `gorm:"column:provider;type:varchar(32);not null;index" json:"provider"`

	SyncMode string `gorm:"column:sync_mode;type:varchar(32);not null;default:'incremental'" json:"sync_mode"` // incremental|full_then_incremental
	Schedule string `gorm:"column:schedule;type:varchar(64);not null;default:'@hourly'" json:"schedule"`
	Status   string `gorm:"column:status;type:varchar(32);not null;default:'active';index" json:"status"`

	Scope      datatypes.JSON `gorm:"column:scope;type:jsonb;default:'{}'" json:"scope,omitempty"`
	LastRunAt  *time.Time     `gorm:"column:last_run_at" json:"last_run_at,omitempty"`
	NextRunAt  *time.Time     `gorm:"column:next_run_at" json:"next_run_at,omitempty"`
	LastOKAt   *time.Time     `gorm:"column:last_ok_at" json:"last_ok_at,omitempty"`
	LastError  string         `gorm:"column:last_error;type:text" json:"last_error,omitempty"`
	LastRunRef string         `gorm:"column:last_run_ref;type:varchar(128)" json:"last_run_ref,omitempty"`

	CreatedBy string `gorm:"column:created_by;type:varchar(128)" json:"created_by,omitempty"`
	UpdatedBy string `gorm:"column:updated_by;type:varchar(128)" json:"updated_by,omitempty"`
}

func (SpaceSyncJob) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableKnowledgeSpaceSyncJobs
}

func (j *SpaceSyncJob) Normalize() {
	if j == nil {
		return
	}
	j.TenantUUID = strings.ToLower(strings.TrimSpace(j.TenantUUID))
	j.Provider = strings.ToLower(strings.TrimSpace(j.Provider))
	j.Status = strings.ToLower(strings.TrimSpace(j.Status))
	j.SyncMode = strings.ToLower(strings.TrimSpace(j.SyncMode))
	j.Schedule = strings.TrimSpace(j.Schedule)
	j.SpaceUUID = strings.TrimSpace(j.SpaceUUID)
	j.ConnectorUUID = strings.TrimSpace(j.ConnectorUUID)
}
