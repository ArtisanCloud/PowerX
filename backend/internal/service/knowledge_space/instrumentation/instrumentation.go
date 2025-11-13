package instrumentation

import (
	"context"
	"time"

	auditsvc "github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	pxlog "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

// Span 表示可关闭的 trace span。
type Span interface {
	End(err error)
}

// Tracer 描述追踪实现。
type Tracer interface {
	StartSpan(ctx context.Context, name string, attrs map[string]string) (context.Context, Span)
}

// MetricsRecorder 定义关键指标。
type MetricsRecorder interface {
	ObserveProvisioningLatency(success bool, duration time.Duration)
	ObserveIngestionCoverage(coverage float64)
	ObserveFusionRollback(duration time.Duration)
	ObserveFeedbackSLA(met bool)
}

// Options 聚合依赖。
type Options struct {
	Tracer  Tracer
	Metrics MetricsRecorder
	Audit   auditsvc.Service
}

// Instrumentation 提供统一的日志、追踪、指标能力。
type Instrumentation struct {
	tracer  Tracer
	metrics MetricsRecorder
	audit   auditsvc.Service
}

// New 构造实例。
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
	return inst
}

// Logger 返回带上下文的日志器。
func (i *Instrumentation) Logger(ctx context.Context) *pxlog.Logger {
	return pxlog.GetGlobalLogger().WithContext(ctx)
}

// Tracer 返回追踪实现。
func (i *Instrumentation) Tracer() Tracer {
	return i.tracer
}

// Metrics 返回指标记录器。
func (i *Instrumentation) Metrics() MetricsRecorder {
	return i.metrics
}

// RecordProvisioning 写入 SLA 指标。
func (i *Instrumentation) RecordProvisioning(success bool, duration time.Duration) {
	if i.metrics != nil {
		i.metrics.ObserveProvisioningLatency(success, duration)
	}
}

// RecordIngestionCoverage 写入覆盖率。
func (i *Instrumentation) RecordIngestionCoverage(coverage float64) {
	if i.metrics != nil {
		i.metrics.ObserveIngestionCoverage(coverage)
	}
}

// RecordFusionRollbackLatency 写入回滚耗时。
func (i *Instrumentation) RecordFusionRollbackLatency(duration time.Duration) {
	if i.metrics != nil {
		i.metrics.ObserveFusionRollback(duration)
	}
}

// RecordFeedbackSLA 记录反馈是否在 SLA 内完成。
func (i *Instrumentation) RecordFeedbackSLA(met bool) {
	if i.metrics != nil {
		i.metrics.ObserveFeedbackSLA(met)
	}
}

// Audit 记录审计事件。
func (i *Instrumentation) Audit(ctx context.Context, evt *dbm.AuditEvent) {
	if i.audit == nil || evt == nil {
		return
	}
	_ = i.audit.Emit(ctx, evt)
}

type noopTracer struct{}

func (noopTracer) StartSpan(ctx context.Context, _ string, _ map[string]string) (context.Context, Span) {
	return ctx, noopSpan{}
}

type noopSpan struct{}

func (noopSpan) End(error) {}

type noopMetrics struct{}

func (noopMetrics) ObserveProvisioningLatency(bool, time.Duration) {}
func (noopMetrics) ObserveIngestionCoverage(float64)               {}
func (noopMetrics) ObserveFusionRollback(time.Duration)            {}
func (noopMetrics) ObserveFeedbackSLA(bool)                        {}
