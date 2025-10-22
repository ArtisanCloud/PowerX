package agent_lifecycle

import (
	"time"

	agentmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	"github.com/google/uuid"
)

// ToolGrant 描述代理可用的授权。
type ToolGrant struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

// Agent 表示对外返回的代理档案。
type Agent struct {
	ID                       uuid.UUID `json:"id"`
	TenantID                 string    `json:"tenant_id"`
	Alias                    string    `json:"alias"`
	DisplayName              string    `json:"display_name"`
	Status                   string    `json:"status"`
	ToolGrants               []ToolGrant
	TelemetryContractVersion string `json:"telemetry_contract_version"`
	DefaultCapacityInstances int32  `json:"default_capacity_instances"`
	MaxCapacityInstances     *int32 `json:"max_capacity_instances,omitempty"`
	CurrentCapacityInstances int32  `json:"current_capacity_instances"`
	EventTopicPrefix         string `json:"event_topic_prefix"`
	NotificationChannel      string `json:"notification_channel,omitempty"`
	Metadata                 map[string]string
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
}

// RegisterInput 注册代理所需信息。
type RegisterInput struct {
	TenantID                 string
	Alias                    string
	DisplayName              string
	ToolGrants               []ToolGrant
	TelemetryContractVersion string
	DefaultCapacityInstances int32
	MaxCapacityInstances     *int32
	EventTopicPrefix         string
	NotificationChannel      string
	Metadata                 map[string]string
	RequestedBy              string
	TraceID                  string
}

// ActivateInput 激活代理的输入。
type ActivateInput struct {
	AgentID     uuid.UUID
	TenantID    string
	Reason      string
	RequestedBy string
	TraceID     string
}

type PauseInput struct {
	AgentID     uuid.UUID
	TenantID    string
	Reason      string
	RequestedBy string
	TraceID     string
}

type ResumeInput struct {
	AgentID     uuid.UUID
	TenantID    string
	Reason      string
	RequestedBy string
	TraceID     string
}

type RetireInput struct {
	AgentID     uuid.UUID
	TenantID    string
	Reason      string
	RequestedBy string
	TraceID     string
}

type ScaleInput struct {
	AgentID     uuid.UUID
	TenantID    string
	Target      int32
	Reason      string
	RequestedBy string
	TraceID     string
}

type LifecycleResult struct {
	Agent *Agent
	Event *agentmodel.AgentLifecycleEventRecord
}
