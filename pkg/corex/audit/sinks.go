package audit

import (
	"context"

	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	pxlog "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"go.uber.org/zap"
)

// LoggerSink：复用你现有 logger（会带 ctx 的链路字段）
type LoggerSink struct{ L *pxlog.Logger }

func (s *LoggerSink) Emit(ctx context.Context, evt *dbm.AuditEvent) error {
	if s == nil || s.L == nil || evt == nil {
		return nil
	}
	s.L.Info(ctx, "audit_event",
		zap.String("source", evt.Source),
		zap.String("operation", evt.Operation),
		zap.String("outcome", evt.Outcome),
		zap.String("severity", evt.Severity),
		zap.String("resource_type", evt.ResourceType),
		zap.String("resource_id", evt.ResourceID),
		zap.String("correlation_id", evt.CorrelationID),
		zap.Uint64("tenant_id", evt.TenantID),
		zap.ByteString("meta", evt.Meta),
	)
	return nil
}

// BusSink（可选）：按需实现你的 Publisher
type EventPublisher interface {
	Publish(ctx context.Context, topic string, payload any) error
}
type BusSink struct {
	Topic     string
	Publisher EventPublisher
}

func (b *BusSink) Emit(ctx context.Context, evt *dbm.AuditEvent) error {
	if b == nil || b.Publisher == nil || evt == nil {
		return nil
	}
	return b.Publisher.Publish(ctx, b.Topic, evt)
}
