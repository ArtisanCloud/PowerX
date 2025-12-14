package tenant

import (
	"context"
	"time"

	router "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/router"
	manager "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/manager"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
)

// capabilityRouter 描述可路由的能力调用服务。
type capabilityRouter interface {
	Invoke(ctx context.Context, req router.InvokeRequest) (router.InvokeResult, error)
}

// ToolGrantChecker 负责校验租户是否持有指定 Tool Grant。
type ToolGrantChecker interface {
	Validate(ctx context.Context, tenantUUID string, grants []string) error
}

// InvokeInput 描述租户发起调用的参数。
type InvokeInput struct {
	TenantUUID     string
	RouteSlug      string
	Channel        string
	Payload        map[string]any
	Context        map[string]any
	IdempotencyKey string
	TraceID        string
	Actor          string
}

// InvokeStatus 定义调用结果状态。
type InvokeStatus string

const (
	InvokeStatusOK          InvokeStatus = "ok"
	InvokeStatusAccepted    InvokeStatus = "accepted"
	InvokeStatusRateLimited InvokeStatus = "rate_limited"
	InvokeStatusDenied      InvokeStatus = "denied"
	InvokeStatusFailed      InvokeStatus = "failed"
)

// RateLimitResult 在限流命中时返回。
type RateLimitResult struct {
	Scope      string
	RetryAfter time.Duration
	Remaining  int64
}

// InvokeResult 返回租户调用结果。
type InvokeResult struct {
	Status             InvokeStatus
	Result             map[string]any
	RoutedCapabilityID string
	RoutedAdapter      string
	TraceID            string
	DispatchedAt       time.Time
	Duration           time.Duration
	ErrorCode          string
	ErrorMessage       string
	RateLimit          *RateLimitResult
}

// Config 描述运行时默认配置。
type Config struct {
	DefaultRateLimit manager.RateLimitPolicy
	EventTopics      manager.EventTopics
}

// EventPublisher 简化事件发布接口，便于替换实现。
type EventPublisher interface {
	event_bus.EventBus
}
