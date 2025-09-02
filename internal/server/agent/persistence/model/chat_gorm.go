package model

import (
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/datatypes"
)

type AgentChatMessage struct {
	coremodel.PowerModel

	// 作用域
	Env      string  `gorm:"size:32;index" json:"-"`
	TenantID *uint64 `gorm:"index" json:"-"`

	// 关联
	SessionID uint64 `gorm:"index;not null" json:"sessionId"`
	AgentID   uint64 `gorm:"index;not null" json:"agentId"`

	// 内容
	Role      string            `gorm:"size:16;index" json:"role"`            // user|assistant|system|tool|summary
	Content   string            `gorm:"type:text" json:"content"`             // Postgres 用 text；MySQL 可改为 longtext
	Format    string            `gorm:"size:16;default:'text'" json:"format"` // text|markdown|json
	Tokens    int               `gorm:"default:0" json:"tokens"`
	SizeBytes int               `gorm:"default:0" json:"sizeBytes"`
	Pinned    bool              `gorm:"default:false;index" json:"pinned"` // 关键消息不参与裁剪
	Meta      datatypes.JSONMap `gorm:"type:jsonb;default:'{}'::jsonb" json:"meta"`
}

func (mdl *AgentChatMessage) TableName() string {
	return coremodel.PowerXSchema + "." + TableAgentChatMessage
}
func (mdl *AgentChatMessage) GetTableName(needFull bool) string {
	if needFull {
		return mdl.TableName()
	}
	return TableAgentChatMessage
}
