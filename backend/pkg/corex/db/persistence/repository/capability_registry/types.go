package capability_registry

import (
	"time"

	"github.com/google/uuid"
)

// Registration 描述能力注册的领域快照。
type Registration struct {
	CapabilityID        string
	TenantUUID          string
	ContractRef         string
	Status              string
	EnvironmentPolicies map[string]EnvironmentPolicy
	Adapters            []AdapterEndpoint
	RoutingPolicy       RoutingPolicy
	FallbackPlan        *FallbackPlan
	ToolGrantIDs        []string
	Version             uint64
	CreatedAt           time.Time
	UpdatedAt           time.Time
	UpdatedBy           string
	PublishedAt         *time.Time
	DisableReason       string
}

// AdapterEndpoint 描述一个注册绑定的适配器端点。
type AdapterEndpoint struct {
	AdapterID      string
	TransportType  string
	Endpoint       string
	ServiceRef     string
	Weight         int
	TimeoutMS      int
	MaxConcurrency *int
	Labels         map[string]string
	Visibility     VisibilityPolicy
	HealthPolicyID *uuid.UUID
	IsActive       bool
}

// VisibilityPolicy 描述适配器在环境与租户维度的可见性。
type VisibilityPolicy struct {
	Environments VisibilityRule `json:"environments,omitempty"`
	Tenants      VisibilityRule `json:"tenants,omitempty"`
}

// VisibilityRule 定义允许/拒绝列表。
type VisibilityRule struct {
	Allow []string `json:"allow,omitempty"`
	Deny  []string `json:"deny,omitempty"`
}

// RoutingPolicy 描述能力的路由策略。
type RoutingPolicy struct {
	ID               uuid.UUID
	Strategy         string
	TenantStrategies map[string]string
	RateLimit        *RateLimit
	FallbackSequence []string
	CooldownSeconds  int
	StickyKeys       []string
	LastUpdatedAt    time.Time
	LastUpdatedBy    string
}

// RateLimit 路由限流配置。
type RateLimit struct {
	Limit         uint32 `json:"limit,omitempty"`
	WindowSeconds uint32 `json:"window_seconds,omitempty"`
}

// FallbackPlan 描述回退策略。
type FallbackPlan struct {
	ID                  uuid.UUID
	PrimaryCapabilityID string
	FallbackTargets     []string
	StaticResponse      *StaticResponse
	TriggerConditions   map[string]interface{}
	NotificationChannel string
}

// StaticResponse 定义静态回退响应。
type StaticResponse struct {
	Payload    map[string]interface{} `json:"payload,omitempty"`
	TTLSeconds uint32                 `json:"ttl_seconds,omitempty"`
}

// EnvironmentPolicy 定义环境覆盖策略。
type EnvironmentPolicy struct {
	IsEnabled bool              `json:"is_enabled"`
	Overrides map[string]string `json:"overrides,omitempty"`
}
