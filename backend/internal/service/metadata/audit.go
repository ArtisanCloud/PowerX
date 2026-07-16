package metadata

import (
	"context"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

func (s *Service) publishAudit(ctx context.Context, event AuditEvent) {
	logCtx := logger.WithLogFields(ctx, map[string]interface{}{
		"trace_id":    reqctx.GetTraceID(ctx),
		"tenant_uuid": event.TenantUUID,
		"operation":   event.Operation,
		"object_type": event.ObjectType,
		"object_uuid": event.ObjectUUID,
		"error_code":  event.ErrorCode,
	})
	logger.InfoF(logCtx, "[metadata] operation_audit")
	if s == nil || s.deps.AuditPublisher == nil {
		return
	}
	_ = s.deps.AuditPublisher.PublishMetadataAudit(ctx, event)
}
