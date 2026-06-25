package model

import (
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type AgentChatContextSummary struct {
	coremodel.PowerModel

	UUID uuid.UUID `gorm:"type:uuid;column:uuid;uniqueIndex;index" json:"uuid"`

	Env        string  `gorm:"size:32;index:agent_ctx_sum_scope,priority:1" json:"-"`
	TenantUUID *string `gorm:"column:tenant_uuid;index:agent_ctx_sum_scope,priority:2" json:"-"`

	SessionID uint64 `gorm:"index:agent_ctx_sum_scope,priority:3;index" json:"sessionId"`
	AgentID   uint64 `gorm:"index" json:"agentId"`
	UserID    uint64 `gorm:"index" json:"userId"`

	SummaryID       string  `gorm:"size:128;uniqueIndex;not null" json:"summaryId"`
	SourceSummaryID *string `gorm:"size:128;index" json:"sourceSummaryId,omitempty"`
	Schema          string  `gorm:"size:128;index" json:"schema"`

	FromMessageID      uint64 `gorm:"index" json:"fromMessageId"`
	ToMessageID        uint64 `gorm:"index" json:"toMessageId"`
	CompressedMessages int    `gorm:"default:0" json:"compressedMessages"`
	RecentMessagesKept int    `gorm:"default:0" json:"recentMessagesKept"`
	CompressionPolicy  string `gorm:"size:64;index" json:"compressionPolicy"`

	SummaryJSON datatypes.JSONMap `gorm:"type:jsonb;default:'{}'::jsonb" json:"summaryJson"`
	SummaryText string            `gorm:"type:text" json:"summaryText"`
	Checksum    string            `gorm:"size:128;index" json:"checksum"`
	ArtifactURI string            `gorm:"type:text" json:"artifactUri,omitempty"`
	Meta        datatypes.JSONMap `gorm:"type:jsonb;default:'{}'::jsonb" json:"meta"`
}

func (mdl *AgentChatContextSummary) TableName() string {
	return coremodel.PowerXSchema + "." + TableAgentContextSummary
}

func (mdl *AgentChatContextSummary) BeforeCreate(tx *gorm.DB) error {
	if mdl.UUID == uuid.Nil {
		mdl.UUID = uuid.New()
	}
	return nil
}

func (mdl *AgentChatContextSummary) GetTableName(needFull bool) string {
	if needFull {
		return mdl.TableName()
	}
	return TableAgentContextSummary
}
