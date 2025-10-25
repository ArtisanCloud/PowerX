package tenant

import (
	"context"
	"fmt"
	"strings"
	"time"

	authorization "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/authorization"
	manager "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/manager"
	"github.com/google/uuid"
)

func convertPolicy(policy manager.RateLimitPolicy, fallback manager.RateLimitPolicy) authorization.RateLimitPolicy {
	if policy.Limit == 0 {
		policy.Limit = fallback.Limit
	}
	if policy.Burst == 0 {
		policy.Burst = policy.Limit
	}
	if policy.WindowSeconds <= 0 {
		policy.WindowSeconds = fallback.WindowSeconds
	}
	scope := strings.TrimSpace(policy.Scope)
	if scope == "" {
		scope = fallback.Scope
	}
	return authorization.RateLimitPolicy{
		Limit:    policy.Limit,
		Burst:    policy.Burst,
		Interval: time.Duration(policy.WindowSeconds) * time.Second,
	}
}

func rateLimitScope(policy manager.RateLimitPolicy, fallback manager.RateLimitPolicy) string {
	scope := strings.TrimSpace(policy.Scope)
	if scope == "" {
		scope = fallback.Scope
	}
	if scope == "" {
		return "per_route_per_tenant"
	}
	return scope
}

func buildRateLimitKey(scope string, tenantID string, routeID uuid.UUID) string {
	switch scope {
	case "per_tenant":
		return fmt.Sprintf("tenant:%s", tenantID)
	case "per_route":
		return fmt.Sprintf("route:%s", routeID)
	default:
		return fmt.Sprintf("route:%s:tenant:%s", routeID, tenantID)
	}
}

func (s *Service) checkRateLimit(ctx context.Context, route manager.Route, policy manager.RateLimitPolicy) (authorization.RateLimitResult, string, error) {
	if s.rateLimiter == nil {
		return authorization.RateLimitResult{Allowed: true, Remaining: -1}, rateLimitScope(policy, s.config.DefaultRateLimit), nil
	}
	scope := rateLimitScope(policy, s.config.DefaultRateLimit)
	key := buildRateLimitKey(scope, route.TenantID, route.RouteID)
	result, err := s.rateLimiter.Allow(ctx, key, convertPolicy(policy, s.config.DefaultRateLimit))
	return result, scope, err
}
