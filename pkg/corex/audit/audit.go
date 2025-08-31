package audit

import (
	"context"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"time"
)

type Auditor interface {
	LogAPI(ctx context.Context, methodPath string, status int, latency time.Duration)
	LogBusPublish(ctx context.Context, topic string, subCount int)
	LogBusDeliver(ctx context.Context, topic, pluginID string, status int, err string)
	LogRBAC(ctx context.Context, subject string, resource string, action string, allow bool)
}

type Noop struct{}

func (Noop) LogAPI(context.Context, string, int, time.Duration)         {}
func (Noop) LogBusPublish(context.Context, string, int)                 {}
func (Noop) LogBusDeliver(context.Context, string, string, int, string) {}
func (Noop) LogRBAC(context.Context, string, string, string, bool)      {}

// GetTraceID 从 context 里拿 TraceID（没有就返回空串）
func GetTraceID(ctx context.Context) string {
	if v, ok := ctx.Value(reqctx.TraceIDKey).(string); ok {
		return v
	}
	return ""
}

// WithTraceID 往 context 里写 TraceID
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, reqctx.TraceIDKey, traceID)
}
