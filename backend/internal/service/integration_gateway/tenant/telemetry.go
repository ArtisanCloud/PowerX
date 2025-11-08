package tenant

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/instrumentation"
)

// telemetry 记录租户调用指标与日志。
type telemetry struct {
	inst                 *instrumentation.Instrumentation
	invocationsTotal     atomic.Int64
	rateLimitHitsTotal   atomic.Int64
	lastInvocationStatus atomic.Value
}

func newTelemetry(inst *instrumentation.Instrumentation) *telemetry {
	if inst == nil {
		inst = instrumentation.NewInstrumentation(nil)
	}
	return &telemetry{inst: inst}
}

func (t *telemetry) Instrumentation() *instrumentation.Instrumentation {
	if t == nil {
		return instrumentation.NewInstrumentation(nil)
	}
	return t.inst
}

func (t *telemetry) ObserveInvocation(ctx context.Context, status InvokeStatus, duration time.Duration) {
	if t == nil {
		return
	}
	t.invocationsTotal.Add(1)
	t.lastInvocationStatus.Store(status)

	logger := t.inst.Logger(ctx)
	logger.InfoF(ctx, "[integration_gateway.tenant] invocation status=%s duration=%s", status, duration)
}

func (t *telemetry) ObserveRateLimit(ctx context.Context, scope string) {
	if t == nil {
		return
	}
	t.rateLimitHitsTotal.Add(1)
	logger := t.inst.Logger(ctx)
	logger.WarnF(ctx, "[integration_gateway.tenant] rate limit hit scope=%s", scope)
}

func (t *telemetry) Snapshot() (invocations int64, rateLimits int64, status InvokeStatus) {
	if t == nil {
		return 0, 0, InvokeStatus("")
	}
	invocations = t.invocationsTotal.Load()
	rateLimits = t.rateLimitHitsTotal.Load()
	if value := t.lastInvocationStatus.Load(); value != nil {
		if st, ok := value.(InvokeStatus); ok {
			status = st
		}
	}
	return
}
