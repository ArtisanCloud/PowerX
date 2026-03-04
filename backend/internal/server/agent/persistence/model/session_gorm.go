package model

import (
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type AgentChatSession struct {
	coremodel.PowerModel

	UUID uuid.UUID `gorm:"type:uuid;column:uuid;uniqueIndex;index" json:"uuid"`

	// 作用域（与现有 Agent 表一致）
	Env        string  `gorm:"size:32;index:agent_sess_scope" json:"-"`
	TenantUUID *string `gorm:"column:tenant_uuid;index:agent_sess_scope" json:"-"`

	// 归属
	AgentID uint64 `gorm:"index;not null" json:"agentId"`
	UserID  uint64 `gorm:"size:64;index" json:"userId"` // 发起/归属的用户ID（按需调整类型）

	// 展示/策略
	Title     string `gorm:"size:255" json:"title"`
	Singleton bool   `gorm:"default:false;index" json:"singleton"` // 若 Agent 走单例会话
	TTLDays   int    `gorm:"default:3" json:"ttlDays"`             // 会话级 TTL（天）
	MaxKB     int    `gorm:"default:200" json:"maxKB"`             // 会话累计大小上限（KB）
	MaxTokens int    `gorm:"default:3000" json:"maxTokens"`        // 会话累计 token 上限（近似）

	// 滚动摘要（可选）
	Summary   string     `gorm:"type:text" json:"summary"`
	SummaryAt *time.Time `json:"summaryAt,omitempty"`

	// 状态 & 统计
	Status    string            `gorm:"size:16;default:'active';index" json:"status"` // active|archived|deleted
	LatestAt  *time.Time        `gorm:"index" json:"latestAt,omitempty"`              // 最近一条消息时间
	ExpiredAt *time.Time        `gorm:"index" json:"expiredAt,omitempty"`             // 到期时间（根据 TTL 计算）
	Meta      datatypes.JSONMap `gorm:"type:jsonb;default:'{}'::jsonb" json:"meta"`
}

func (mdl *AgentChatSession) TableName() string {
	return coremodel.PowerXSchema + "." + TableAgentChatSession
}

func (mdl *AgentChatSession) BeforeCreate(tx *gorm.DB) error {
	if mdl.UUID == uuid.Nil {
		mdl.UUID = uuid.New()
	}
	return nil
}
func (mdl *AgentChatSession) GetTableName(needFull bool) string {
	if needFull {
		return mdl.TableName()
	}
	return TableAgentChatSession
}
