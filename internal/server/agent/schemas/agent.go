package schemas

import (
	"github.com/ArtisanCloud/PowerX/internal/server/agent/config"
	"time"
)

type AgentStatus string

const (
	StatusInit    AgentStatus = "init"
	StatusReady   AgentStatus = "ready"
	StatusRunning AgentStatus = "running"
	StatusStopped AgentStatus = "stopped"
	StatusError   AgentStatus = "error"
)

type AgentInfo struct {
	AgentID     string              `json:"agent_id"`
	Name        string              `json:"name,omitempty"`
	Description string              `json:"description,omitempty"`
	Status      string              `json:"status"`
	Config      *config.AgentConfig `json:"config"`
	CreatedAt   time.Time           `json:"created_at"`
	UpdatedAt   time.Time           `json:"updated_at"`
	LastBeatAt  time.Time           `json:"last_beat_at"`
	Runtime     *Runtime            `json:"runtime"`
	Extras      map[string]string   `json:"extras,omitempty"`
}

type AgentMeta struct {
	ID          string `json:"id"`
	TenantID    string `json:"tenant_id,omitempty"`
	UserID      string `json:"user_id,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`

	// 这里没有你原先的 Config 类型，就用最小必要字段（如 FlowID）。
	FlowID string `json:"flow_id,omitempty"`

	Status     AgentStatus       `json:"status"`
	Extras     map[string]string `json:"extras,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
	LastBeatAt time.Time         `json:"last_beat_at"`
}
