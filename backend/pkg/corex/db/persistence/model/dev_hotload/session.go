package devhotload

import (
	"strings"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// DevHotloadSession captures Dev API session metadata.
type DevHotloadSession struct {
	coremodel.PowerUUIDModel

	PluginID        string         `gorm:"column:plugin_id;type:varchar(128);not null;index:idx_dev_hotload_plugin_tenant,priority:1" json:"plugin_id"`
	TenantUUID      string         `gorm:"column:tenant_uuid;type:varchar(128);not null;index:idx_dev_hotload_plugin_tenant,priority:2" json:"tenant_uuid"`
	DeveloperID     uint64         `gorm:"column:developer_id;not null" json:"developer_id"`
	BuildHash       string         `gorm:"column:build_hash;type:varchar(128)" json:"build_hash,omitempty"`
	EntryPoints     datatypes.JSON `gorm:"column:entry_points;type:jsonb;default:'[]'" json:"entry_points,omitempty"`
	Manifest        datatypes.JSON `gorm:"column:manifest;type:jsonb;default:'{}'" json:"manifest,omitempty"`
	Metadata        datatypes.JSON `gorm:"column:metadata;type:jsonb;default:'{}'" json:"metadata,omitempty"`
	ReloadToken     string         `gorm:"column:reload_token;type:text;not null" json:"reload_token"`
	Status          string         `gorm:"column:status;type:varchar(32);not null;index" json:"status"`
	SandboxEndpoint string         `gorm:"column:sandbox_endpoint;type:text" json:"sandbox_endpoint,omitempty"`
	LogURL          string         `gorm:"column:log_url;type:text" json:"log_url,omitempty"`
	WatchFileLimit  int            `gorm:"column:watch_file_limit;default:0" json:"watch_file_limit,omitempty"`
	ExpiresAt       time.Time      `gorm:"column:expires_at;index" json:"expires_at"`
	EndedAt         *time.Time     `gorm:"column:ended_at" json:"ended_at,omitempty"`
	TerminationNote string         `gorm:"column:termination_note;type:text" json:"termination_note,omitempty"`
	AuditID         uuid.UUID      `gorm:"column:audit_id;type:uuid" json:"audit_id"`
}

func (DevHotloadSession) TableName() string {
	schema := strings.TrimSpace(coremodel.PowerXSchema)
	if schema == "" {
		return coremodel.TableDevHotloadSessions
	}
	return schema + "." + coremodel.TableDevHotloadSessions
}

const (
	DevHotloadSessionStatusPending    = "pending"
	DevHotloadSessionStatusActive     = "active"
	DevHotloadSessionStatusTerminated = "terminated"
	DevHotloadSessionStatusExpired    = "expired"
)
