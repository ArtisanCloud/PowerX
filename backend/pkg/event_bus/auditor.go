package event_bus

import "context"

type Auditor interface {
	LogPublish(ctx context.Context, topic string, subCount int)
	LogDeliver(ctx context.Context, topic, pluginID string, status int, errMsg string)
}

type NoopAuditor struct{}

func (NoopAuditor) LogPublish(context.Context, string, int)                 {}
func (NoopAuditor) LogDeliver(context.Context, string, string, int, string) {}
