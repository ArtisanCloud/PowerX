package router

import (
	"context"
	"sync/atomic"
	"time"

	domain "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/domain"
)

// MetricsRecorder 定义 Router 领域需要上报的指标。
type MetricsRecorder interface {
	ObserveInvocation(ctx context.Context, mode string, capabilityID, tenantUUID, adapterID, transport string, latency time.Duration, fallback bool, err error)
	ObserveFallback(ctx context.Context, capabilityID, tenantUUID, reason string)
	ObserveHealthReport(ctx context.Context, capabilityID, tenantUUID, adapterID, status string, err error)
}

type noopMetricsRecorder struct{}

func (noopMetricsRecorder) ObserveInvocation(context.Context, string, string, string, string, string, time.Duration, bool, error) {
}

func (noopMetricsRecorder) ObserveFallback(context.Context, string, string, string) {}

func (noopMetricsRecorder) ObserveHealthReport(context.Context, string, string, string, string, error) {
}

// RouterMetrics 为路由器提供基于内存的指标实现。
type RouterMetrics struct {
	inst *domain.Instrumentation

	totalInvocations    atomic.Uint64
	fallbackInvocations atomic.Uint64
	errorCount          atomic.Uint64
	healthReports       atomic.Uint64
	unhealthyReports    atomic.Uint64

	maxLatencyNS         atomic.Int64
	maxFallbackLatencyNS atomic.Int64

	fallbackThreshold time.Duration
}

// NewRouterMetrics 创建指标记录器。
func NewRouterMetrics(inst *domain.Instrumentation) *RouterMetrics {
	if inst == nil {
		inst = domain.NewInstrumentation(nil)
	}
	return &RouterMetrics{
		inst:              inst,
		fallbackThreshold: 500 * time.Millisecond,
	}
}

// ObserveInvocation 记录调用指标。
func (m *RouterMetrics) ObserveInvocation(ctx context.Context, mode string, capabilityID, tenantUUID, adapterID, transport string, latency time.Duration, fallback bool, err error) {
	m.totalInvocations.Add(1)
	m.updateMax(&m.maxLatencyNS, latency)

	if err != nil {
		m.errorCount.Add(1)
		m.inst.Logger(ctx).WarnF(ctx, "[router.metrics] invocation failed: tenant_uuid=%s capability=%s mode=%s err=%v", tenantUUID, capabilityID, mode, err)
	}

	if fallback {
		m.fallbackInvocations.Add(1)
		m.updateMax(&m.maxFallbackLatencyNS, latency)
		if latency > m.fallbackThreshold {
			m.inst.Logger(ctx).WarnF(ctx, "[router.metrics] fallback latency exceeds target: tenant_uuid=%s capability=%s latency=%s threshold=%s", tenantUUID, capabilityID, latency, m.fallbackThreshold)
		}
	}
}

// ObserveFallback 记录触发降级的原因。
func (m *RouterMetrics) ObserveFallback(ctx context.Context, capabilityID, tenantUUID, reason string) {
	m.inst.Logger(ctx).InfoF(ctx, "[router.metrics] fallback triggered: tenant_uuid=%s capability=%s reason=%s", tenantUUID, capabilityID, reason)
}

// ObserveHealthReport 记录健康状态上报。
func (m *RouterMetrics) ObserveHealthReport(ctx context.Context, capabilityID, tenantUUID, adapterID, status string, err error) {
	m.healthReports.Add(1)
	if err != nil {
		m.inst.Logger(ctx).WarnF(ctx, "[router.metrics] health report failed: tenant_uuid=%s capability=%s adapter=%s err=%v", tenantUUID, capabilityID, adapterID, err)
		return
	}
	if status != "" && status != "healthy" {
		m.unhealthyReports.Add(1)
		m.inst.Logger(ctx).WarnF(ctx, "[router.metrics] adapter marked unhealthy: tenant_uuid=%s capability=%s adapter=%s status=%s", tenantUUID, capabilityID, adapterID, status)
	}
}

// Snapshot 返回当前指标快照。
func (m *RouterMetrics) Snapshot() RouterMetricsSnapshot {
	total := m.totalInvocations.Load()
	fallbacks := m.fallbackInvocations.Load()
	errors := m.errorCount.Load()
	health := m.healthReports.Load()
	unhealthy := m.unhealthyReports.Load()

	var fallbackRate float64
	if total > 0 {
		fallbackRate = float64(fallbacks) / float64(total)
	}

	return RouterMetricsSnapshot{
		Invocations:         total,
		FallbackInvocations: fallbacks,
		ErrorCount:          errors,
		FallbackRate:        fallbackRate,
		MaxLatency:          time.Duration(m.maxLatencyNS.Load()),
		MaxFallbackLatency:  time.Duration(m.maxFallbackLatencyNS.Load()),
		HealthReports:       health,
		UnhealthyReports:    unhealthy,
	}
}

// RouterMetricsSnapshot 描述 Router 的关键指标。
type RouterMetricsSnapshot struct {
	Invocations         uint64
	FallbackInvocations uint64
	ErrorCount          uint64
	FallbackRate        float64
	MaxLatency          time.Duration
	MaxFallbackLatency  time.Duration
	HealthReports       uint64
	UnhealthyReports    uint64
}

func (m *RouterMetrics) updateMax(target *atomic.Int64, latency time.Duration) {
	ns := latency.Nanoseconds()
	if ns <= 0 {
		return
	}
	for {
		prev := target.Load()
		if ns <= prev {
			return
		}
		if target.CompareAndSwap(prev, ns) {
			return
		}
	}
}
