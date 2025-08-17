package audit

import (
	"context"
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
