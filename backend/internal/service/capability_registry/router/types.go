package router

import (
	"context"
	"time"

	domain "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/domain"
	capabilityRegistry "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/registry"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"gorm.io/gorm"
)

const (
	// EventRouterInvoked 表示路由成功选路事件名。
	EventRouterInvoked = "capability.router.invoked"
	// EventRouterFallback 表示路由触发 fallback 的事件名。
	EventRouterFallback = "capability.router.fallback"
)

// InvokeRequest 描述 Router 调用输入，仅允许传入租户 UUID。
type InvokeRequest struct {
	CapabilityID      string
	TenantUUID        string
	Payload           []byte
	Timeout           time.Duration
	StickyKey         string
	PreferredProtocol string
}

// InvokeResult 描述 Router 调用结果。
type InvokeResult struct {
	AdapterID    string
	Endpoint     string
	Transport    string
	FallbackUsed bool
	Payload      []byte
	Latency      time.Duration
	Error        error
	Labels       map[string]string
}

// ReportHealthInput 描述健康上报。
type ReportHealthInput struct {
	CapabilityID string
	TenantUUID   string
	AdapterID    string
	Status       string
	Reason       string
	Failures     uint32
}

// HealthRepository 定义健康结果持久化接口。
type HealthRepository interface {
	SaveProbeResult(ctx context.Context, db *gorm.DB, result HealthProbeRecord) error
	GetLatest(ctx context.Context, db *gorm.DB, capabilityID, tenantUUID string) ([]HealthProbeRecord, error)
}

// HealthProbeRecord 用于存储探测结果。
type HealthProbeRecord struct {
	CapabilityID string
	TenantUUID   string
	AdapterID    string
	Status       string
	Reason       string
	Failures     uint32
	NextRetryAt  time.Time
	UpdatedAt    time.Time
}

// ServiceOptions 注入 Router 服务依赖。
type ServiceOptions struct {
	DB                 *gorm.DB
	RegistryRepository capabilityRegistry.Repository
	HealthRepository   HealthRepository
	EventBus           event_bus.EventBus
	Instrumentation    *domain.Instrumentation
	Metrics            MetricsRecorder
	Clock              func() time.Time
}

// 以下类型别名暴露 Registry 结构以便上层调用传入覆盖信息。
type (
	Registration      = capabilityRegistry.Registration
	AdapterEndpoint   = capabilityRegistry.AdapterEndpoint
	RoutingPolicy     = capabilityRegistry.RoutingPolicy
	FallbackPlan      = capabilityRegistry.FallbackPlan
	StaticResponse    = capabilityRegistry.StaticResponse
	RateLimit         = capabilityRegistry.RateLimit
	EnvironmentPolicy = capabilityRegistry.EnvironmentPolicy
	VisibilityPolicy  = capabilityRegistry.VisibilityPolicy
	VisibilityRule    = capabilityRegistry.VisibilityRule
)
