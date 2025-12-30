package http

import (
	"context"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// RecoveryMiddleware 捕获 panic 并返回统一错误
func RecoveryMiddleware() gin.HandlerFunc {
	return gin.RecoveryWithWriter(gin.DefaultWriter)
}

// RequestLoggingMiddleware 记录每次请求的基础信息，并把 context 中的 trace_id/tenant 注入日志字段（假设 logging 包支持上下文）
func RequestLoggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		latency := time.Since(start)
		status := c.Writer.Status()
		tenantUUID := reqctx.GetTenantUUID(c.Request.Context())
		traceID := audit.GetTraceID(c.Request.Context())
		logger.Info(c.Request.Context(), "http_request",
			zap.String("method", c.Request.Method),
			zap.String("path", c.FullPath()),
			zap.Int("status", status),
			zap.Int64("latency_ms", latency.Milliseconds()),
			zap.String("tenant_uuid", tenantUUID),
			zap.String("trace_id", traceID),
		)
	}
}

// TraceInjectionMiddleware 确保每个请求都有 trace_id（从 header 继承或新建）并注入 context
func TraceInjectionMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID := c.GetHeader("X-Trace-ID")
		if traceID == "" {
			traceID = uuid.NewString()
		}
		// 将 trace_id 放进 context 便于以后取
		ctx := c.Request.Context()
		ctx = contextWithTraceID(ctx, traceID)
		c.Request = c.Request.WithContext(ctx)
		c.Writer.Header().Set("X-Trace-ID", traceID)
		c.Next()
	}
}

func contextWithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, reqctx.TraceIDKey, traceID)
}

// FeatureInjectionMiddleware 示例：把一些 header 或版本信息注入 context 或请求中
func FeatureInjectionMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 例如：从 header 拿 feature flag source 写入 context，或者注入 request_id
		c.Next()
	}
}
