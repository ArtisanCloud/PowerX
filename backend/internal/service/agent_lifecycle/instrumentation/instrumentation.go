package instrumentation

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"

	auditsvc "github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	pxlog "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

const (
	contextTraceIDKey    = "agent_trace_id"
	contextTenantUUIDKey = "agent_tenant_uuid"
)

// Span 定义可结束的追踪跨度。
type Span interface {
	End(err error)
}

// Tracer 描述可被注入的追踪实现。
type Tracer interface {
	StartSpan(ctx context.Context, name string, attributes map[string]string) (context.Context, Span)
}

// MetricsRecorder 表示观测指标的记录器。
type MetricsRecorder interface {
	IncLifecycleTransition(eventType, toStatus string)
	ObserveHealthScore(status string, score float64)
	ObserveProcessingLatency(operation string, duration time.Duration)
	ObserveHealthLatency(status string, duration time.Duration)
}

// Options 聚合构建 Instrumentation 所需依赖。
type Options struct {
	Tracer  Tracer
	Metrics MetricsRecorder
	Audit   auditsvc.Service
	// AlertCooldown 定义同一代理同一状态重复告警的冷却时间。
	AlertCooldown time.Duration
}

// Instrumentation 为生命周期域统一提供日志、追踪与审计能力。
type Instrumentation struct {
	tracer  Tracer
	metrics MetricsRecorder
	audit   auditsvc.Service
	alerts  *AlertTracker
}

// New 构建 Instrumentation。
func New(opts Options) *Instrumentation {
	inst := &Instrumentation{
		tracer:  opts.Tracer,
		metrics: opts.Metrics,
		audit:   opts.Audit,
	}
	if inst.tracer == nil {
		inst.tracer = noopTracer{}
	}
	if inst.metrics == nil {
		inst.metrics = noopMetrics{}
	}
	if opts.AlertCooldown <= 0 {
		opts.AlertCooldown = 2 * time.Minute
	}
	inst.alerts = NewAlertTracker(opts.AlertCooldown)
	return inst
}

// Logger 返回带上下文的全局日志实例。
func (i *Instrumentation) Logger(ctx context.Context) *pxlog.Logger {
	return pxlog.GetGlobalLogger().WithContext(ctx)
}

// Tracer 返回配置的追踪实现。
func (i *Instrumentation) Tracer() Tracer {
	return i.tracer
}

// Metrics 返回指标记录器。
func (i *Instrumentation) Metrics() MetricsRecorder {
	return i.metrics
}

// AllowHealthAlert 用于判断是否可以发送告警，已考虑节流。
func (i *Instrumentation) AllowHealthAlert(agentID, status string, now time.Time) bool {
	if i.alerts == nil {
		return true
	}
	return i.alerts.Try(agentID, status, now)
}

// EnsureTraceContext 确保上下文带有 trace_id。
func EnsureTraceContext(ctx context.Context) (context.Context, string) {
	if ctx == nil {
		ctx = context.Background()
	}
	if traceID, ok := ctx.Value(contextTraceIDKey).(string); ok && traceID != "" {
		return ctx, traceID
	}
	traceID := uuid.NewString()
	return context.WithValue(ctx, contextTraceIDKey, traceID), traceID
}

// WithTenant 将租户信息写入上下文。
func WithTenant(ctx context.Context, tenantUUID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, contextTenantUUIDKey, tenantUUID)
}

// SpanAttributes 合成追踪属性。
func SpanAttributes(ctx context.Context, extra map[string]string) map[string]string {
	attrs := make(map[string]string, len(extra)+2)
	for k, v := range extra {
		if v != "" {
			attrs[k] = v
		}
	}
	if traceID, ok := ctx.Value(contextTraceIDKey).(string); ok && traceID != "" {
		attrs["trace.id"] = traceID
	}
	if tenantUUID, ok := ctx.Value(contextTenantUUIDKey).(string); ok && tenantUUID != "" {
		attrs["tenant.uuid"] = tenantUUID
	}
	return attrs
}

// RecordLifecycleTransition 记录生命周期状态变更。
func (i *Instrumentation) RecordLifecycleTransition(ctx context.Context, eventType, toStatus string, latency time.Duration) {
	if i.metrics != nil {
		i.metrics.IncLifecycleTransition(eventType, toStatus)
		i.metrics.ObserveProcessingLatency(eventType, latency)
	}
}

// RecordHealthSnapshot 记录健康评分观测。
func (i *Instrumentation) RecordHealthSnapshot(ctx context.Context, status string, score float64) {
	if i.metrics != nil {
		i.metrics.ObserveHealthScore(status, score)
	}
}

// RecordHealthLatency 记录健康计算耗时。
func (i *Instrumentation) RecordHealthLatency(ctx context.Context, status string, latency time.Duration) {
	if i.metrics != nil {
		i.metrics.ObserveHealthLatency(status, latency)
	}
}

// AuditLifecycleEvent 写入审计事件。
func (i *Instrumentation) AuditLifecycleEvent(ctx context.Context, tenantUUID, agentID, operation, outcome string, meta map[string]any) {
	if i.audit == nil {
		return
	}
	ctx, traceID := EnsureTraceContext(ctx)
	payload := marshalMeta(meta)
	_ = i.audit.Emit(ctx, &dbm.AuditEvent{
		OccurredAt:    time.Now().UTC(),
		TenantUUID:    strings.TrimSpace(tenantUUID),
		CorrelationID: traceID,
		Source:        "agent.lifecyle",
		Operation:     operation,
		ResourceType:  "agent.profile",
		ResourceID:    agentID,
		Outcome:       outcome,
		Severity:      severityFromOutcome(outcome),
		Meta:          datatypes.JSON(payload),
	})
}

func marshalMeta(meta map[string]any) []byte {
	if len(meta) == 0 {
		return nil
	}
	b, err := json.Marshal(meta)
	if err != nil {
		return nil
	}
	return b
}

func severityFromOutcome(outcome string) string {
	switch strings.ToUpper(outcome) {
	case "SUCCESS":
		return "INFO"
	case "DENIED":
		return "WARN"
	default:
		return "ERROR"
	}
}

type noopTracer struct{}

func (noopTracer) StartSpan(ctx context.Context, _ string, _ map[string]string) (context.Context, Span) {
	return ctx, noopSpan{}
}

type noopSpan struct{}

func (noopSpan) End(error) {}

type noopMetrics struct{}

func (noopMetrics) IncLifecycleTransition(string, string)          {}
func (noopMetrics) ObserveHealthScore(string, float64)             {}
func (noopMetrics) ObserveProcessingLatency(string, time.Duration) {}
func (noopMetrics) ObserveHealthLatency(string, time.Duration)     {}
