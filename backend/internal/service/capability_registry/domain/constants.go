package domain

import "errors"

// RegistrationStatus 表示能力注册快照的生命周期状态。
type RegistrationStatus string

const (
	RegistrationStatusDraft      RegistrationStatus = "draft"
	RegistrationStatusPublished  RegistrationStatus = "published"
	RegistrationStatusDeprecated RegistrationStatus = "deprecated"
	RegistrationStatusDisabled   RegistrationStatus = "disabled"
)

// RoutingStrategy 定义选路策略。
type RoutingStrategy string

const (
	RoutingStrategyWeightedRoundRobin RoutingStrategy = "weighted_round_robin"
	RoutingStrategyPriority           RoutingStrategy = "priority"
	RoutingStrategySticky             RoutingStrategy = "sticky"
)

// FallbackKind 表示回退执行方式。
type FallbackKind string

const (
	FallbackKindAdapter    FallbackKind = "adapter"
	FallbackKindCapability FallbackKind = "capability"
	FallbackKindStatic     FallbackKind = "static_response"
)

// HealthState 描述适配器或能力的健康状态。
type HealthState string

const (
	HealthStateHealthy   HealthState = "healthy"
	HealthStateDegraded  HealthState = "degraded"
	HealthStateUnhealthy HealthState = "unhealthy"
)

var (
	// ErrRegistrationNotFound 表示能力注册不存在。
	ErrRegistrationNotFound = errors.New("capability registry: registration not found")
	// ErrAdapterUnavailable 表示没有可用适配器。
	ErrAdapterUnavailable = errors.New("capability registry: no healthy adapter available")
	// ErrFallbackPlanMissing 表示缺少可用回退策略。
	ErrFallbackPlanMissing = errors.New("capability registry: fallback plan missing")
	// ErrSandboxNotAllowed 表示调用方无权限执行 sandbox 操作。
	ErrSandboxNotAllowed = errors.New("capability registry: sandbox operation not allowed")
)

// IsValidRegistrationStatus 判断状态是否受支持。
func IsValidRegistrationStatus(status RegistrationStatus) bool {
	switch status {
	case RegistrationStatusDraft,
		RegistrationStatusPublished,
		RegistrationStatusDeprecated,
		RegistrationStatusDisabled:
		return true
	default:
		return false
	}
}

// IsValidRoutingStrategy 判断选路策略是否受支持。
func IsValidRoutingStrategy(strategy RoutingStrategy) bool {
	switch strategy {
	case RoutingStrategyWeightedRoundRobin,
		RoutingStrategyPriority,
		RoutingStrategySticky:
		return true
	default:
		return false
	}
}

// IsValidFallbackKind 判断回退类型是否受支持。
func IsValidFallbackKind(kind FallbackKind) bool {
	switch kind {
	case FallbackKindAdapter,
		FallbackKindCapability,
		FallbackKindStatic:
		return true
	default:
		return false
	}
}
