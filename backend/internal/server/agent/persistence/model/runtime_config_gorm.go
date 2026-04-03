package model

import (
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"time"
)

type AgentRuntimeConfig struct {
	coremodel.PowerModel

	UUID uuid.UUID `gorm:"type:uuid;column:uuid;uniqueIndex;index" json:"uuid"`

	Env        string  `gorm:"size:32;index:agent_runtime_cfg_scope,priority:1" json:"env"`
	TenantUUID *string `gorm:"column:tenant_uuid;index:agent_runtime_cfg_scope,priority:2" json:"tenant_uuid,omitempty"`
	Scope      string  `gorm:"size:16;default:'tenant';index:agent_runtime_cfg_scope,priority:3" json:"scope"` // tenant|system

	ConfigType string `gorm:"size:64;default:'context_optimizer';index:agent_runtime_cfg_scope,priority:4" json:"config_type"`
	Version    int    `gorm:"index:agent_runtime_cfg_scope,priority:5" json:"version"`
	Status     string `gorm:"size:16;default:'draft';index:agent_runtime_cfg_scope,priority:6" json:"status"` // draft|published|archived

	ConfigJSON datatypes.JSONMap `gorm:"type:jsonb;default:'{}'::jsonb" json:"config"`

	ChangeReason string     `gorm:"type:text" json:"change_reason,omitempty"`
	CreatedBy    uint64     `gorm:"default:0" json:"created_by,omitempty"`
	PublishedBy  uint64     `gorm:"default:0" json:"published_by,omitempty"`
	PublishedAt  *time.Time `json:"published_at,omitempty"`
}

func (mdl *AgentRuntimeConfig) TableName() string {
	return coremodel.PowerXSchema + "." + TableAgentRuntimeConfig
}

func (mdl *AgentRuntimeConfig) BeforeCreate(tx *gorm.DB) error {
	if mdl.UUID == uuid.Nil {
		mdl.UUID = uuid.New()
	}
	return nil
}
