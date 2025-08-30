// ctx_correlation.go
package audit

import "context"

type ctxKey string

const ctxKeyCorrelationID ctxKey = "correlation_id"

// WithCorrelationID 返回带有 correlation_id 的新 context
func WithCorrelationID(ctx context.Context, cid string) context.Context {
	return context.WithValue(ctx, ctxKeyCorrelationID, cid)
}

// CorrelationIDFromContext 从 ctx 取 correlation_id（可能为空串）
func CorrelationIDFromContext(ctx context.Context) string {
	if v := ctx.Value(ctxKeyCorrelationID); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
