package capability_registry

import (
	"context"
	"errors"
	"strings"

	eventbus "github.com/ArtisanCloud/PowerX/internal/event_bus"
	capmetrics "github.com/ArtisanCloud/PowerX/internal/observability/metrics"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
)

type selectorHooks struct {
	bus     event_bus.EventBus
	metrics *capmetrics.CapabilityRegistryMetrics
}

func newSelectorHooks(bus event_bus.EventBus, metrics *capmetrics.CapabilityRegistryMetrics) *selectorHooks {
	if bus == nil && metrics == nil {
		return nil
	}
	if metrics == nil {
		metrics = capmetrics.NewCapabilityRegistryMetrics(nil)
	}
	return &selectorHooks{
		bus:     bus,
		metrics: metrics,
	}
}

func (h *selectorHooks) RecordFailure(ctx context.Context, req CapabilityInvokeRequest, err error) {
	if h == nil {
		return
	}
	if h.bus != nil {
		payload := map[string]any{
			"capability_id":      strings.TrimSpace(req.CapabilityID),
			"tenant_uuid":        strings.TrimSpace(req.TenantUUID),
			"intent":             strings.TrimSpace(req.Intent),
			"tool_scope":         strings.TrimSpace(req.ToolScope),
			"preferred_protocol": strings.TrimSpace(req.PreferredProtocol),
			"trace_id":           strings.TrimSpace(req.TraceID),
			"status":             statusForSelectorError(err),
		}
		if len(req.ToolGrantIDs) > 0 {
			payload["tool_grant_ids"] = append([]string(nil), req.ToolGrantIDs...)
		}
		if err != nil {
			payload["error"] = err.Error()
		}
		h.bus.Publish(eventbus.TopicIntegrationGatewayInvocationFailed, payload, ctx)
	}

	if h.metrics != nil {
		labels := capmetrics.CapabilityInvocationLabels{
			CapabilityID: strings.TrimSpace(req.CapabilityID),
			Protocol:     strings.TrimSpace(req.PreferredProtocol),
			TenantUUID:   strings.TrimSpace(req.TenantUUID),
			TraceID:      strings.TrimSpace(req.TraceID),
			Result:       capmetrics.ResultFailed,
		}
		sample := capmetrics.CapabilityInvocationSample{
			Labels: labels,
			Err:    err,
		}
		h.metrics.ObserveInvocation(ctx, sample)
	}
}

func statusForSelectorError(err error) string {
	switch {
	case errors.Is(err, ErrSelectorCapabilityForbidden):
		return "denied"
	case errors.Is(err, ErrSelectorCapabilityRequired):
		return "not_found"
	case errors.Is(err, ErrSelectorTenantRequired):
		return "invalid_request"
	case errors.Is(err, ErrSelectorSafeModeActive):
		return "safe_mode"
	case errors.Is(err, ErrSelectorToolGrantRequired):
		return "tool_grant_missing"
	case errors.Is(err, ErrSelectorFeatureFlagMissing):
		return "feature_flag_missing"
	case errors.Is(err, ErrSelectorUnavailable):
		return "unavailable"
	default:
		return "failed"
	}
}
