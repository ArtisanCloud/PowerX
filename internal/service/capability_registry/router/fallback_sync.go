package router

import (
	"context"
	"strings"
	"time"

	registry "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/registry"
)

// syncFallbackState 将 Router 的 fallback 结果同步到内存状态与健康存储。
func (s *Service) syncFallbackState(ctx context.Context, reg registry.Registration, reason string) error {
	if len(reg.Adapters) == 0 {
		return nil
	}

	now := s.now()
	cooldown := time.Duration(reg.RoutingPolicy.CooldownSeconds) * time.Second
	if cooldown <= 0 {
		cooldown = 60 * time.Second
	}

	routeKey := routingKey(reg.TenantID, reg.CapabilityID)
	failures := s.markAdaptersUnhealthy(routeKey, reg.Adapters, reason, now)
	nextRetry := now.Add(cooldown)

	if s.healthRepo == nil {
		return nil
	}

	var firstErr error
	for _, ep := range reg.Adapters {
		record := HealthProbeRecord{
			CapabilityID: reg.CapabilityID,
			TenantID:     reg.TenantID,
			AdapterID:    ep.AdapterID,
			Status:       "unhealthy",
			Reason:       reason,
			Failures:     failures[ep.AdapterID],
			NextRetryAt:  nextRetry,
			UpdatedAt:    now,
		}
		if err := s.healthRepo.SaveProbeResult(ctx, nil, record); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// markAdaptersUnhealthy 将指定能力的所有适配器标记为不可用，并返回最新失败次数。
func (s *Service) markAdaptersUnhealthy(routeKey string, adapters []registry.AdapterEndpoint, reason string, now time.Time) map[string]uint32 {
	failures := make(map[string]uint32, len(adapters))

	s.stateMu.Lock()
	adapterMap := s.adapterState[routeKey]
	if adapterMap == nil {
		adapterMap = make(map[string]healthState, len(adapters))
		s.adapterState[routeKey] = adapterMap
	}
	for _, ep := range adapters {
		state := adapterMap[ep.AdapterID]
		state.status = "unhealthy"
		state.reason = reason
		if state.failures < ^uint32(0) {
			state.failures++
		}
		state.updated = now
		adapterMap[ep.AdapterID] = state
		failures[ep.AdapterID] = state.failures
	}
	s.stateMu.Unlock()

	for _, ep := range adapters {
		s.clearStickyAdapter(routeKey, ep.AdapterID)
	}

	return failures
}

// clearStickyAdapter 清理指向指定适配器的 sticky 记录。
func (s *Service) clearStickyAdapter(routeKey, adapterID string) {
	s.stickyMu.Lock()
	defer s.stickyMu.Unlock()

	for key, stuck := range s.stickyMap {
		if stuck == adapterID && strings.HasPrefix(key, routeKey+"::") {
			delete(s.stickyMap, key)
		}
	}
}

func fallbackReasonFromSelection(selection adapterSelection) string {
	if selection.fallbackUsed {
		return "fallback: plan"
	}
	return "fallback: no_adapter"
}
