package setting

import (
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"time"
)

// PluginCapabilityApproval is an independently approved grant, not a manifest
// declaration. Its UUID is the durable approval reference in credential data.
type PluginCapabilityApproval struct {
	coremodel.PowerUUIDModel
	TenantUUID         string     `gorm:"type:uuid;not null;uniqueIndex:uk_plugin_capability_approval,priority:1,where:status = 'active' AND deleted_at IS NULL"`
	PluginID           string     `gorm:"size:128;not null;uniqueIndex:uk_plugin_capability_approval,priority:2"`
	CapabilityID       string     `gorm:"size:256;not null;uniqueIndex:uk_plugin_capability_approval,priority:3"`
	ApprovedByUserUUID uuid.UUID  `gorm:"type:uuid;not null"`
	Status             string     `gorm:"size:16;not null"`
	RevokedByUserUUID  *uuid.UUID `gorm:"type:uuid"`
	RevokedAt          *time.Time
}

func (*PluginCapabilityApproval) TableName() string {
	return coremodel.PowerXSchema + "." + TablePluginCapabilityApproval
}

func (m *PluginCapabilityApproval) BeforeCreate(*gorm.DB) error {
	if m.UUID == uuid.Nil {
		m.UUID = uuid.New()
	}
	return nil
}
