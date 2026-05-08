package audit

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
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
	meta := decodeMeta(evt.Meta)
	requestID := readContextString(ctx, "request_id")
	if requestID == "" {
		requestID = readContextString(ctx, "powerx.request_id")
	}
	if requestID == "" {
		requestID = strings.TrimSpace(meta["request_id"])
	}
	traceID := strings.TrimSpace(reqctx.GetTraceID(ctx))
	if traceID == "" {
		traceID = strings.TrimSpace(meta["trace_id"])
	}
	pluginID := readContextString(ctx, "plugin_id")
	if pluginID == "" {
		pluginID = strings.TrimSpace(meta["plugin_id"])
	}
	if pluginID == "" {
		pluginID = extractPluginIDFromResourceID(evt.ResourceID)
	}

	s.L.Info(ctx, "audit_event",
		zap.String("source", evt.Source),
		zap.String("operation", evt.Operation),
		zap.String("outcome", evt.Outcome),
		zap.String("severity", evt.Severity),
		zap.String("resource_type", evt.ResourceType),
		zap.String("resource_id", evt.ResourceID),
		zap.String("correlation_id", evt.CorrelationID),
		zap.String("tenant_uuid", evt.TenantUUID),
		zap.String("request_id", requestID),
		zap.String("trace_id", traceID),
		zap.String("plugin_id", pluginID),
		zap.ByteString("meta", evt.Meta),
	)
	return nil
}

func readContextString(ctx context.Context, key string) string {
	if ctx == nil || strings.TrimSpace(key) == "" {
		return ""
	}
	if v, ok := ctx.Value(key).(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func decodeMeta(meta []byte) map[string]string {
	if len(meta) == 0 {
		return map[string]string{}
	}
	var raw map[string]any
	if err := json.Unmarshal(meta, &raw); err != nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out
}

func extractPluginIDFromResourceID(resourceID string) string {
	raw := strings.TrimSpace(resourceID)
	if raw == "" {
		return ""
	}
	// expected format: "<METHOD> <PATH>"
	if idx := strings.IndexByte(raw, ' '); idx >= 0 && idx+1 < len(raw) {
		raw = raw[idx+1:]
	}
	return reqctx.ResolvePluginIDFromPath(raw)
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
