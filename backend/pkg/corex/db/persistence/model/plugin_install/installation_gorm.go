package plugininstall

import (
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/datatypes"
	"time"
)

const TableInstallations = "plugin_installations"

// Installation stores the accepted intent and installation checkpoints. Delivery,
// lease ownership and retries belong exclusively to the formal TaskBus.
type Installation struct {
	model.PowerUUIDModel
	TenantUUID        string         `gorm:"type:uuid;not null;uniqueIndex:uk_plugin_install_request,priority:1;index:idx_plugin_install_owner,priority:1"`
	ActorUUID         string         `gorm:"size:36;not null"`
	ActorID           uint64         `gorm:"not null;uniqueIndex:uk_plugin_install_request,priority:2;index:idx_plugin_install_owner,priority:2"`
	RequestID         string         `gorm:"type:uuid;not null;uniqueIndex:uk_plugin_install_request,priority:3"`
	InstanceID        string         `gorm:"size:128;not null;index"`
	RequestDigest     string         `gorm:"size:64;not null"`
	PrivateRequest    []byte         `gorm:"type:bytea;not null" json:"-"`
	SourceKind        string         `gorm:"size:16;not null"`
	SourceDigest      string         `gorm:"size:64"`
	SourceBytes       int64          `gorm:"not null;default:0"`
	PluginID          string         `gorm:"size:256"`
	Version           string         `gorm:"size:128"`
	InstalledRevision string         `gorm:"size:64"`
	Status            string         `gorm:"size:32;not null;index:idx_plugin_install_owner,priority:3"`
	Phase             string         `gorm:"size:32;not null"`
	Revision          int64          `gorm:"not null;default:1"`
	ErrorCode         string         `gorm:"size:128"`
	FailureDetails    datatypes.JSON `gorm:"type:jsonb"`
	CompletedAt       *time.Time
}

func (*Installation) TableName() string { return model.PowerXSchema + "." + TableInstallations }
