// pkg/corex/db/persistence/model/setting/plugin_instance_config.go
package setting

import (
	"strings"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	PluginInstanceStatusAvailable          = "available"
	PluginInstanceStatusSubscribed         = "subscribed"
	PluginInstanceStatusEnabled            = "enabled"
	PluginInstanceStatusDisabled           = "disabled"
	PluginInstanceStatusDrainingRequested  = "draining_requested"
	PluginInstanceStatusDisabledByPlatform = "disabled_by_platform"
	PluginInstanceStatusDrained            = "drained"
	PluginInstanceStatusExpired            = "expired"

	PluginDrainJobScopePlugin        = "plugin"
	PluginDrainJobScopePluginVersion = "plugin_version"

	PluginDrainJobStatusRequested        = "requested"
	PluginDrainJobStatusBlockingNewUsage = "blocking_new_usage"
	PluginDrainJobStatusDraining         = "draining"
	PluginDrainJobStatusReadyToUninstall = "ready_to_uninstall"
	PluginDrainJobStatusCompleted        = "completed"
	PluginDrainJobStatusFailed           = "failed"
	PluginDrainJobStatusCancelled        = "cancelled"
)

// 插件“租户态”的启停与参数；版本/安装真源仍由 JSONRegistry 管
type PluginInstanceConfig struct {
	coremodel.PowerModel

	TenantUUID string `gorm:"column:tenant_uuid;type:varchar(128);not null;uniqueIndex:uk_plugincfg_tpk,priority:1" json:"tenant_uuid"`
	PluginID   string `gorm:"column:plugin_id;type:varchar(128);not null;uniqueIndex:uk_plugincfg_tpk,priority:2" json:"plugin_id"`
	Key        string `gorm:"column:key;type:varchar(128);not null;uniqueIndex:uk_plugincfg_tpk,priority:3" json:"key"`

	ValueJSON datatypes.JSON `gorm:"column:value_json;type:jsonb" json:"value_json,omitempty"`

	Enabled          bool       `gorm:"column:enabled;default:true;index" json:"enabled"`
	Status           string     `gorm:"column:status;type:varchar(32);not null;default:'enabled';index" json:"status"`
	DrainJobID       string     `gorm:"column:drain_job_id;type:varchar(64);index" json:"drain_job_id,omitempty"`
	DrainRequestedAt *time.Time `gorm:"column:drain_requested_at" json:"drain_requested_at,omitempty"`
	DrainedAt        *time.Time `gorm:"column:drained_at" json:"drained_at,omitempty"`
}

func (m *PluginInstanceConfig) TableName() string {
	return coremodel.PowerXSchema + "." + TablePluginInstanceConfig
}
func (m *PluginInstanceConfig) GetTableName(full bool) string {
	if full {
		return m.TableName()
	}
	return TablePluginInstanceConfig
}

func (m *PluginInstanceConfig) BeforeCreate(tx *gorm.DB) error {
	m.normalizeStatus()
	return nil
}

func (m *PluginInstanceConfig) BeforeSave(tx *gorm.DB) error {
	m.normalizeStatus()
	return nil
}

func (m *PluginInstanceConfig) normalizeStatus() {
	m.Status = NormalizePluginInstanceStatus(m.Status, m.Enabled)
}

type PluginDrainJob struct {
	coremodel.PowerUUIDModel

	JobID               string         `gorm:"column:job_id;type:varchar(64);not null;uniqueIndex" json:"job_id"`
	PluginID            string         `gorm:"column:plugin_id;type:varchar(128);not null;index:idx_plugin_drain_target,priority:1" json:"plugin_id"`
	Version             string         `gorm:"column:version;type:varchar(64);index:idx_plugin_drain_target,priority:2" json:"version,omitempty"`
	Scope               string         `gorm:"column:scope;type:varchar(32);not null;default:'plugin'" json:"scope"`
	Status              string         `gorm:"column:status;type:varchar(32);not null;default:'requested';index" json:"status"`
	Reason              string         `gorm:"column:reason;type:text" json:"reason,omitempty"`
	RequestedByRootUser uint64         `gorm:"column:requested_by_root_user_id;index" json:"requested_by_root_user_id"`
	AffectedTenantCount int64          `gorm:"column:affected_tenant_count;not null;default:0" json:"affected_tenant_count"`
	DrainedTenantCount  int64          `gorm:"column:drained_tenant_count;not null;default:0" json:"drained_tenant_count"`
	LastBlockerJSON     datatypes.JSON `gorm:"column:last_blocker_json;type:jsonb" json:"last_blocker_json,omitempty"`
	CompletedAt         *time.Time     `gorm:"column:completed_at" json:"completed_at,omitempty"`
}

func (m *PluginDrainJob) TableName() string {
	return coremodel.PowerXSchema + "." + TablePluginDrainJob
}

func (m *PluginDrainJob) GetTableName(full bool) string {
	if full {
		return m.TableName()
	}
	return TablePluginDrainJob
}

func (m *PluginDrainJob) BeforeCreate(tx *gorm.DB) error {
	if m.UUID == uuid.Nil {
		m.UUID = uuid.New()
	}
	m.normalize()
	if strings.TrimSpace(m.JobID) == "" {
		m.JobID = m.UUID.String()
	}
	return nil
}

func (m *PluginDrainJob) BeforeSave(tx *gorm.DB) error {
	m.normalize()
	return nil
}

func (m *PluginDrainJob) normalize() {
	m.PluginID = strings.TrimSpace(m.PluginID)
	m.Version = strings.TrimSpace(m.Version)
	m.Scope = NormalizePluginDrainJobScope(m.Scope, m.Version)
	m.Status = NormalizePluginDrainJobStatus(m.Status)
}

func NormalizePluginInstanceStatus(status string, enabled bool) string {
	switch strings.TrimSpace(status) {
	case PluginInstanceStatusAvailable,
		PluginInstanceStatusSubscribed,
		PluginInstanceStatusEnabled,
		PluginInstanceStatusDisabled,
		PluginInstanceStatusDrainingRequested,
		PluginInstanceStatusDisabledByPlatform,
		PluginInstanceStatusDrained,
		PluginInstanceStatusExpired:
		return strings.TrimSpace(status)
	default:
		if enabled {
			return PluginInstanceStatusEnabled
		}
		return PluginInstanceStatusDisabled
	}
}

func NormalizePluginDrainJobScope(scope string, version string) string {
	switch strings.TrimSpace(scope) {
	case PluginDrainJobScopePlugin, PluginDrainJobScopePluginVersion:
		return strings.TrimSpace(scope)
	default:
		if strings.TrimSpace(version) != "" {
			return PluginDrainJobScopePluginVersion
		}
		return PluginDrainJobScopePlugin
	}
}

func NormalizePluginDrainJobStatus(status string) string {
	switch strings.TrimSpace(status) {
	case PluginDrainJobStatusRequested,
		PluginDrainJobStatusBlockingNewUsage,
		PluginDrainJobStatusDraining,
		PluginDrainJobStatusReadyToUninstall,
		PluginDrainJobStatusCompleted,
		PluginDrainJobStatusFailed,
		PluginDrainJobStatusCancelled:
		return strings.TrimSpace(status)
	default:
		return PluginDrainJobStatusRequested
	}
}
