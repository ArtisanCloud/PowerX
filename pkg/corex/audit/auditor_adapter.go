// pkg/corex/audit/auditor_adapter.go
package audit

import (
	"context"
	"time"
)

type Service interface {
	Emit(ctx context.Context, evt *AuditEvent) error // 你新的审计落库+分发入口
}

type serviceAuditor struct {
	svc Service
}

func NewAuditor(svc Service) Auditor {
	return &serviceAuditor{svc: svc}
}

func (a *serviceAuditor) LogAPI(ctx context.Context, methodPath string, status int, latency time.Duration) {
	evt := NewEventFromCtx(ctx).
		Source("http").
		Operation("API_CALL").
		Resource("core.api", methodPath, "").
		Outcome(httpOutcome(status)).
		Severity(sevByHTTP(status)).
		Meta(map[string]any{"status": status, "latency_ms": latency.Milliseconds()})
	_ = a.svc.Emit(ctx, evt)
}

func (a *serviceAuditor) LogBusPublish(ctx context.Context, topic string, subCount int) {
	evt := NewEventFromCtx(ctx).
		Source("bus").
		Operation("BUS_PUBLISH").
		Resource("core.bus.topic", topic, "").
		Outcome(OutcomeSuccess).
		Meta(map[string]any{"subscribers": subCount})
	_ = a.svc.Emit(ctx, evt)
}

func (a *serviceAuditor) LogBusDeliver(ctx context.Context, topic, pluginID string, status int, err string) {
	evt := NewEventFromCtx(ctx).
		Source("bus").
		Operation("BUS_DELIVER").
		Resource("plugin", pluginID, "").
		Outcome(httpOutcome(status)).
		Severity(sevByHTTP(status)).
		Meta(map[string]any{"topic": topic, "status": status, "error": err})
	_ = a.svc.Emit(ctx, evt)
}

func (a *serviceAuditor) LogRBAC(ctx context.Context, subject, resource, action string, allow bool) {
	out := OutcomeDenied
	if allow {
		out = OutcomeSuccess
	}
	evt := NewEventFromCtx(ctx).
		Source("rbac").
		Operation("RBAC_CHECK").
		Resource("rbac.resource", resource, "").
		Outcome(out).
		Severity(SeverityInfo).
		Meta(map[string]any{"subject": subject, "action": action})
	_ = a.svc.Emit(ctx, evt)
}
