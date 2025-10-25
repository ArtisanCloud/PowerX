package workflow

import "context"

// MetricsRecorder 定义工作流运行态需要的核心观测指标。
type MetricsRecorder interface {
	ObserveRetryScheduled(ctx context.Context, tenantID uint64, stepType string)
	ObserveCompensationTriggered(ctx context.Context, tenantID uint64, stepType string)
	ObserveCompensationResult(ctx context.Context, tenantID uint64, stepType string, success bool)
}

type noopMetricsRecorder struct{}

func (noopMetricsRecorder) ObserveRetryScheduled(context.Context, uint64, string)           {}
func (noopMetricsRecorder) ObserveCompensationTriggered(context.Context, uint64, string) {}
func (noopMetricsRecorder) ObserveCompensationResult(context.Context, uint64, string, bool) {
}
