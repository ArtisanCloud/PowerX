package workflow

import "context"

// MetricsRecorder 定义工作流运行态需要的核心观测指标。
type MetricsRecorder interface {
	ObserveRetryScheduled(ctx context.Context, tenantUUID string, stepType string)
	ObserveCompensationTriggered(ctx context.Context, tenantUUID string, stepType string)
	ObserveCompensationResult(ctx context.Context, tenantUUID string, stepType string, success bool)
}

type noopMetricsRecorder struct{}

func (noopMetricsRecorder) ObserveRetryScheduled(context.Context, string, string)           {}
func (noopMetricsRecorder) ObserveCompensationTriggered(context.Context, string, string) {}
func (noopMetricsRecorder) ObserveCompensationResult(context.Context, string, string, bool) {
}
