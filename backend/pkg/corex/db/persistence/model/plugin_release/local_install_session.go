package plugin_release

import (
	"strings"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/datatypes"
)

// LocalInstallSession tracks px-plugin dev/watch hotload sessions.
type LocalInstallSession struct {
	coremodel.PowerUUIDModel

	TenantUUID          string         `gorm:"column:tenant_uuid;type:varchar(128);index:idx_plugin_release_local_session_tenant,priority:1" json:"tenant_uuid"`
	DeveloperMemberUUID string         `gorm:"column:developer_member_uuid;type:uuid;not null;index:idx_plugin_release_local_session_tenant,priority:2" json:"developer_member_uuid"`
	ArtifactURI         string         `gorm:"column:artifact_uri;type:text;not null" json:"artifact_uri"`
	Status              string         `gorm:"column:status;type:varchar(32);not null;default:'in_progress';index" json:"status"`
	LogPointers         datatypes.JSON `gorm:"column:log_pointers;type:jsonb;default:'{}'" json:"log_pointers,omitempty"`
	FeatureFlags        datatypes.JSON `gorm:"column:feature_flags;type:jsonb;default:'[]'" json:"feature_flags,omitempty"`
	ExpiredAt           *time.Time     `gorm:"column:expired_at" json:"expired_at,omitempty"`
}

func (LocalInstallSession) TableName() string {
	schema := strings.TrimSpace(coremodel.PowerXSchema)
	if schema == "" {
		return coremodel.TablePluginReleaseLocalInstallSessions
	}
	return schema + "." + coremodel.TablePluginReleaseLocalInstallSessions
}

const (
	LocalInstallStatusInProgress = "in_progress"
	LocalInstallStatusSuccess    = "success"
	LocalInstallStatusFailed     = "failed"
)
