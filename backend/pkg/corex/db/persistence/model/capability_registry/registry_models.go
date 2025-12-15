package capability_registry

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

// CapabilityRegistration 记录能力注册的版本化快照。
type CapabilityRegistration struct {
	coremodel.PowerModel

	CapabilityID        string         `gorm:"column:capability_id;type:varchar(128);not null;index:idx_registry_capability_tenant_version,priority:1" json:"capability_id"`
	TenantUUID          string         `gorm:"column:tenant_uuid;type:char(36);not null;index:idx_registry_capability_tuuid_version,priority:1;index:idx_registry_capability_tuuid_status" json:"tenant_uuid"`
	ContractRef         string         `gorm:"column:contract_ref;type:varchar(256);not null" json:"contract_ref"`
	Status              string         `gorm:"column:status;type:varchar(32);not null;index:idx_registry_capability_tenant_status" json:"status"`
	EnvironmentPolicies datatypes.JSON `gorm:"column:environment_policies;type:jsonb;default:'{}'" json:"environment_policies,omitempty"`
	ToolGrantIDs        datatypes.JSON `gorm:"column:tool_grant_ids;type:jsonb;default:'[]'" json:"tool_grant_ids,omitempty"`
	Version             uint64         `gorm:"column:version;not null;index:idx_registry_capability_tenant_version,priority:3" json:"version"`
	UpdatedBy           string         `gorm:"column:updated_by;type:varchar(128)" json:"updated_by,omitempty"`
	PublishedAt         *time.Time     `gorm:"column:published_at" json:"published_at,omitempty"`
	DisableReason       string         `gorm:"column:disable_reason;type:text" json:"disable_reason,omitempty"`

	RoutingPolicyID uuid.UUID  `gorm:"column:routing_policy_id;type:uuid;not null" json:"routing_policy_id"`
	FallbackPlanID  *uuid.UUID `gorm:"column:fallback_plan_id;type:uuid" json:"fallback_plan_id,omitempty"`

	RoutingPolicy *RoutingPolicy    `gorm:"foreignKey:UUID;references:RoutingPolicyID" json:"routing_policy,omitempty"`
	FallbackPlan  *FallbackPlan     `gorm:"foreignKey:UUID;references:FallbackPlanID" json:"fallback_plan,omitempty"`
	Adapters      []AdapterEndpoint `gorm:"foreignKey:RegistrationID;references:ID" json:"adapters,omitempty"`
}

func (CapabilityRegistration) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableCapabilityRegistryRegistration
}

// AdapterEndpoint 记录能力注册下的适配器端点信息。
type AdapterEndpoint struct {
	coremodel.PowerModel

	RegistrationID uint64         `gorm:"column:registration_id;not null;index:idx_registry_adapter_registration" json:"registration_id"`
	CapabilityID   string         `gorm:"column:capability_id;type:varchar(128);not null;index:idx_registry_adapter_capability" json:"capability_id"`
	TenantUUID     string         `gorm:"column:tenant_uuid;type:char(36);not null;index:idx_registry_adapter_tuuid" json:"tenant_uuid"`
	AdapterID      string         `gorm:"column:adapter_id;type:varchar(128);not null;index:idx_registry_adapter_unique,priority:1" json:"adapter_id"`
	TransportType  string         `gorm:"column:transport_type;type:varchar(32);not null" json:"transport_type"`
	Endpoint       string         `gorm:"column:endpoint;type:text" json:"endpoint,omitempty"`
	ServiceRef     string         `gorm:"column:service_ref;type:text" json:"service_ref,omitempty"`
	Weight         int            `gorm:"column:weight;not null" json:"weight"`
	TimeoutMS      int            `gorm:"column:timeout_ms;not null" json:"timeout_ms"`
	MaxConcurrency *int           `gorm:"column:max_concurrency" json:"max_concurrency,omitempty"`
	Labels         datatypes.JSON `gorm:"column:labels;type:jsonb;default:'{}'" json:"labels,omitempty"`
	Visibility     datatypes.JSON `gorm:"column:visibility;type:jsonb;default:'{}'" json:"visibility,omitempty"`
	HealthPolicyID *uuid.UUID     `gorm:"column:health_policy_id;type:uuid" json:"health_policy_id,omitempty"`
	IsActive       bool           `gorm:"column:is_active;default:true" json:"is_active"`
}

func (AdapterEndpoint) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableCapabilityRegistryAdapterEndpoint
}

// RoutingPolicy 描述注册使用的路由策略。
type RoutingPolicy struct {
	coremodel.PowerUUIDModel

	Strategy         string         `gorm:"column:strategy;type:varchar(64);not null" json:"strategy"`
	TenantStrategies datatypes.JSON `gorm:"column:tenant_strategies;type:jsonb;default:'{}'" json:"tenant_strategies,omitempty"`
	RateLimit        datatypes.JSON `gorm:"column:rate_limit;type:jsonb;default:'{}'" json:"rate_limit,omitempty"`
	FallbackSequence datatypes.JSON `gorm:"column:fallback_sequence;type:jsonb;default:'[]'" json:"fallback_sequence,omitempty"`
	CooldownSeconds  int            `gorm:"column:cooldown_seconds;not null" json:"cooldown_seconds"`
	StickyKeys       datatypes.JSON `gorm:"column:sticky_keys;type:jsonb;default:'[]'" json:"sticky_keys,omitempty"`
}

func (RoutingPolicy) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableCapabilityRegistryRoutingPolicy
}

// FallbackPlan 记录能力的回退策略。
type FallbackPlan struct {
	coremodel.PowerUUIDModel

	PrimaryCapabilityID string         `gorm:"column:primary_capability_id;type:varchar(128);not null" json:"primary_capability_id"`
	FallbackTargets     datatypes.JSON `gorm:"column:fallback_targets;type:jsonb;default:'[]'" json:"fallback_targets,omitempty"`
	StaticResponse      datatypes.JSON `gorm:"column:static_response;type:jsonb;default:'{}'" json:"static_response,omitempty"`
	TriggerConditions   datatypes.JSON `gorm:"column:trigger_conditions;type:jsonb;default:'{}'" json:"trigger_conditions,omitempty"`
	NotificationChannel string         `gorm:"column:notification_channel;type:varchar(64)" json:"notification_channel,omitempty"`
}

func (FallbackPlan) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableCapabilityRegistryFallbackPlan
}

// HealthProbeResult 保存适配器的健康探测结果。
type HealthProbeResult struct {
	AdapterID        string    `gorm:"column:adapter_id;type:varchar(128);not null;primaryKey"`
	ProbeWindowStart time.Time `gorm:"column:probe_window_start;not null;primaryKey"`

	Status       string  `gorm:"column:status;type:varchar(32);not null"`
	Reason       string  `gorm:"column:reason;type:text" json:"reason,omitempty"`
	Confidence   float64 `gorm:"column:confidence" json:"confidence"`
	NextRetryAt  time.Time
	FailureCount int    `gorm:"column:failure_count"`
	Version      uint64 `gorm:"column:version"`

	CreatedAt time.Time      `gorm:"column:created_at"`
	UpdatedAt time.Time      `gorm:"column:updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

func (HealthProbeResult) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableCapabilityRegistryHealthProbeResult
}

// DiscoveryCacheEntry 记录下发给客户端的快照缓存。
type DiscoveryCacheEntry struct {
	TenantUUID      string    `gorm:"column:tenant_uuid;type:char(36);not null;index:idx_registry_discovery_tuuid_capability_client,priority:1"`
	CapabilityID    string    `gorm:"column:capability_id;type:varchar(128);not null;primaryKey"`
	ClientID        string    `gorm:"column:client_id;type:varchar(128);not null;primaryKey"`
	SnapshotVersion uint64    `gorm:"column:snapshot_version;not null"`
	PayloadHash     string    `gorm:"column:payload_hash;type:varchar(128);not null"`
	ExpiresAt       time.Time `gorm:"column:expires_at;not null"`
	IssuedAt        time.Time `gorm:"column:issued_at;not null"`
	Source          string    `gorm:"column:source;type:varchar(32);not null"`
	PolicyDigest    string    `gorm:"column:policy_digest;type:varchar(128)"`

	CreatedAt time.Time      `gorm:"column:created_at"`
	UpdatedAt time.Time      `gorm:"column:updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

func (DiscoveryCacheEntry) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableCapabilityRegistryDiscoveryCache
}
