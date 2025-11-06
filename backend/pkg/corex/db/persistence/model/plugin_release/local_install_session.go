package plugin_release

import (
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/datatypes"
)

// LocalInstallSession tracks px-plugin dev/watch hotload sessions.
type LocalInstallSession struct {
	coremodel.PowerUUIDModel

	TenantID     uint64         `gorm:"column:tenant_id;not null;index:idx_plugin_release_local_session_tenant,priority:1" json:"tenant_id"`
	DeveloperID  uint64         `gorm:"column:developer_id;not null;index:idx_plugin_release_local_session_tenant,priority:2" json:"developer_id"`
	ArtifactURI  string         `gorm:"column:artifact_uri;type:text;not null" json:"artifact_uri"`
	Status       string         `gorm:"column:status;type:varchar(32);not null;default:'in_progress';index" json:"status"`
	LogPointers  datatypes.JSON `gorm:"column:log_pointers;type:jsonb;default:'{}'" json:"log_pointers,omitempty"`
	FeatureFlags datatypes.JSON `gorm:"column:feature_flags;type:jsonb;default:'[]'" json:"feature_flags,omitempty"`
	ExpiredAt    *time.Time     `gorm:"column:expired_at;type:timestamptz" json:"expired_at,omitempty"`
}

func (LocalInstallSession) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TablePluginReleaseLocalInstallSessions
}

const (
	LocalInstallStatusInProgress = "in_progress"
	LocalInstallStatusSuccess    = "success"
	LocalInstallStatusFailed     = "failed"
)
