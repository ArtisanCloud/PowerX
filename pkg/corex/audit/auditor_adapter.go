package audit

import (
	"context"
	"time"

	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
)

// 兼容你已存在的 Auditor 门面：LogAPI/LogBus*/LogRBAC → 直接构建 dbm.AuditEvent

type ServicePort interface {
	Emit(ctx context.Context, evt *dbm.AuditEvent) error
}

type serviceAuditor struct{ svc ServicePort }

func NewAuditor(svc ServicePort) Auditor { return &serviceAuditor{svc: svc} }

func (a *serviceAuditor) LogAPI(ctx context.Context, methodPath string, status int, latency time.Duration) {
	_ = a.svc.Emit(ctx, &dbm.AuditEvent{
		OccurredAt:   time.Now(),
		Source:       "http",
		Operation:    "API_CALL",
		ResourceType: "core.api",
		ResourceID:   methodPath,
		Outcome:      httpOutcome(status),
		Severity:     sevByHTTP(status),
		Meta:         mustJSON(map[string]any{"status": status, "latency_ms": latency.Milliseconds()}),
	})
}

func (a *serviceAuditor) LogBusPublish(ctx context.Context, topic string, subCount int) {
	_ = a.svc.Emit(ctx, &dbm.AuditEvent{
		OccurredAt:   time.Now(),
		Source:       "bus",
		Operation:    "BUS_PUBLISH",
		ResourceType: "core.bus.topic",
		ResourceID:   topic,
		Outcome:      "SUCCESS",
		Severity:     "INFO",
		Meta:         mustJSON(map[string]any{"subscribers": subCount}),
	})
}

func (a *serviceAuditor) LogBusDeliver(ctx context.Context, topic, pluginID string, status int, err string) {
	_ = a.svc.Emit(ctx, &dbm.AuditEvent{
		OccurredAt:   time.Now(),
		Source:       "bus",
		Operation:    "BUS_DELIVER",
		ResourceType: "plugin",
		ResourceID:   pluginID,
		Outcome:      httpOutcome(status),
		Severity:     sevByHTTP(status),
		Meta:         mustJSON(map[string]any{"topic": topic, "status": status, "error": err}),
	})
}

func (a *serviceAuditor) LogRBAC(ctx context.Context, subject, resource, action string, allow bool) {
	out := "DENIED"
	if allow {
		out = "SUCCESS"
	}
	_ = a.svc.Emit(ctx, &dbm.AuditEvent{
		OccurredAt:   time.Now(),
		Source:       "rbac",
		Operation:    "RBAC_CHECK",
		ResourceType: "rbac.resource",
		ResourceID:   resource,
		Outcome:      out,
		Severity:     "INFO",
		Meta:         mustJSON(map[string]any{"subject": subject, "action": action}),
	})
}

func httpOutcome(code int) string {
	switch {
	case code >= 200 && code < 400:
		return "SUCCESS"
	case code == 403:
		return "DENIED"
	default:
		return "FAILED"
	}
}
func sevByHTTP(code int) string {
	switch {
	case code >= 500:
		return "ERROR"
	case code >= 400:
		return "WARN"
	default:
		return "INFO"
	}
}
