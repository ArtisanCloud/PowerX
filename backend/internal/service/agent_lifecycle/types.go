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

// ManifestRegistrationInput 描述插件 manifest 自动注册所需字段。
type ManifestRegistrationInput struct {
	PluginID                 string
	PluginVersion            string
	ManifestVersion          string
	TenantID                 string
	Alias                    string
	DisplayName              string
	ToolGrants               []ToolGrant
	TelemetryContractVersion string
	DefaultCapacityInstances int32
	MaxCapacityInstances     *int32
	NotificationChannel      string
	Metadata                 map[string]string
	Capabilities             []string
	Permissions              []string
	RateLimits               map[string]int32
	SandboxProfile           string
	Signature                string
	RequestedBy              string
	TraceID                  string
	DryRun                   bool
	SkipSandbox              bool
}

// ManifestRegistrationResult 表示自动注册结果。
type ManifestRegistrationResult struct {
	Agent   *Agent
	Sandbox *SandboxRunResult
	DryRun  bool
}

// SandboxRunInput 描述沙箱执行输入。
type SandboxRunInput struct {
	AgentID       uuid.UUID
	PluginID      string
	PluginVersion string
	Profile       string
	RequestedBy   string
	TraceID       string
	Metadata      map[string]string
}

// SandboxRunResult 描述沙箱执行结果。
type SandboxRunResult struct {
	Status     string
	ReportURL  string
	Profile    string
	ExecutedAt time.Time
	Metrics    map[string]float64
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

// HealthInput 描述健康快照写入参数。
type HealthInput struct {
	AgentID         uuid.UUID
	TenantID        string
	WindowStartedAt time.Time
	WindowDuration  time.Duration
	Metrics         HealthMetricsInput
	Recommendations []string
	Status          string
	TraceID         string
}

// HealthMetricsInput 定义指标输入。
type HealthMetricsInput struct {
	ThroughputPerMin float64
	SuccessRate      float64
	P95LatencyMs     int32
	ResourceUtilPct  float64
	ErrorRate        float64
	AnomalyTraceIDs  []string
}

// HealthSummary 提供对外的健康摘要视图。
type HealthSummary struct {
	AgentID           uuid.UUID
	Status            string
	HealthScore       int32
	UpdatedAt         time.Time
	WindowDurationSec int32
	Metrics           HealthMetricsInput
	Recommendations   []string
}

// SubscriptionConfig 描述订阅配置。
type SubscriptionConfig struct {
	MetricsFilter  []string
	HealthStatuses []string
	UpdatedAt      time.Time
}

// SubscriptionUpdateInput 更新订阅的输入。
type SubscriptionUpdateInput struct {
	AgentID     uuid.UUID
	TenantID    string
	Config      SubscriptionConfig
	RequestedBy string
	TraceID     string
}

// TenantFormInput 描述租户提交表单内容。
type TenantFormInput struct {
	TenantID                 string
	Alias                    string
	DisplayName              string
	Purpose                  string
	PromptTemplate           string
	TelemetryContractVersion string
	ToolGrants               []ToolGrant
	Permissions              []string
	RateLimit                int32
	SandboxProfile           string
	RequestedBy              string
	TraceID                  string
	Metadata                 map[string]string
}

// PolicyConflictInput 传递给策略冲突检测。
type PolicyConflictInput struct {
	TenantID    string
	Alias       string
	Permissions []string
	RateLimit   int32
}

// TenantForm 表示对外返回的租户表单。
type TenantForm struct {
	ID                       uuid.UUID         `json:"id"`
	TenantID                 string            `json:"tenant_id"`
	Alias                    string            `json:"alias"`
	DisplayName              string            `json:"display_name"`
	Purpose                  string            `json:"purpose"`
	PromptTemplate           string            `json:"prompt_template"`
	TelemetryContractVersion string            `json:"telemetry_contract_version"`
	ToolGrants               []ToolGrant       `json:"tool_grants"`
	Permissions              []string          `json:"permissions"`
	RateLimit                int32             `json:"rate_limit"`
	SandboxProfile           string            `json:"sandbox_profile"`
	Status                   string            `json:"status"`
	WorkflowTicketID         string            `json:"workflow_ticket_id"`
	ConflictReasons          []PolicyConflict  `json:"conflict_reasons"`
	RequestedBy              string            `json:"requested_by"`
	ApprovedBy               string            `json:"approved_by,omitempty"`
	ApprovedAt               *time.Time        `json:"approved_at,omitempty"`
	ActivatedAgentID         *uuid.UUID        `json:"activated_agent_id,omitempty"`
	Metadata                 map[string]string `json:"metadata,omitempty"`
	UpdatedAt                time.Time         `json:"updated_at"`
	CreatedAt                time.Time         `json:"created_at"`
}
