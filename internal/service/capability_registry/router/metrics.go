package router

import (
	"context"
	"time"
)

// MetricsRecorder 定义 Router 领域需要上报的指标。
type MetricsRecorder interface {
	ObserveInvocation(ctx context.Context, mode string, capabilityID, tenantID, adapterID, transport string, latency time.Duration, fallback bool, err error)
	ObserveFallback(ctx context.Context, capabilityID, tenantID, reason string)
	ObserveHealthReport(ctx context.Context, capabilityID, tenantID, adapterID, status string, err error)
}

type noopMetricsRecorder struct{}

func (noopMetricsRecorder) ObserveInvocation(context.Context, string, string, string, string, string, time.Duration, bool, error) {
}

func (noopMetricsRecorder) ObserveFallback(context.Context, string, string, string) {}

func (noopMetricsRecorder) ObserveHealthReport(context.Context, string, string, string, string, error) {
}
