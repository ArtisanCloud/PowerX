package http

import (
	"context"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const requestIDContextKey = "powerx.request_id"

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
		requestID, _ := c.Get("request_id")
		requestIDStr, _ := requestID.(string)
		query := sanitizeQuery(c.Request.URL.Query())
		path := c.FullPath()
		if strings.TrimSpace(path) == "" {
			path = c.Request.URL.Path
		}
		pluginID := extractPluginIDFromPath(c.Request.URL.Path)
		logger.Info(c.Request.Context(), "http_request",
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.String("query", query),
			zap.Int("status", status),
			zap.Int64("latency_ms", latency.Milliseconds()),
			zap.String("tenant_uuid", tenantUUID),
			zap.String("trace_id", traceID),
			zap.String("request_id", requestIDStr),
			zap.String("plugin_id", pluginID),
		)
	}
}

func sanitizeQuery(v url.Values) string {
	if len(v) == 0 {
		return ""
	}
	// 避免把用户内容/敏感信息写入日志（SSE 的 q/message、token 等）
	redactKeys := map[string]struct{}{
		"q":              {},
		"message":        {},
		"prompt":         {},
		"system_prompt":  {},
		"systemPrompt":   {},
		"api_key":        {},
		"apiKey":         {},
		"authorization":  {},
		"access_token":   {},
		"refresh_token":  {},
		"token":          {},
		"bearer":         {},
		"password":       {},
		"secret":         {},
		"client_secret":  {},
		"private_key":    {},
		"signature":      {},
		"sig":            {},
		"x-api-key":      {},
		"x_api_key":      {},
		"apikey":         {},
		"openai_api_key": {},
	}

	// clone + redact
	out := make(url.Values, len(v))
	keys := make([]string, 0, len(v))
	for k := range v {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		kl := strings.ToLower(strings.TrimSpace(k))
		if _, ok := redactKeys[kl]; ok {
			out[k] = []string{"<redacted>"}
			continue
		}
		vals := v[k]
		clean := make([]string, 0, len(vals))
		for _, s := range vals {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			// 单个值也做截断，避免日志过长
			if len(s) > 200 {
				s = s[:200] + "…"
			}
			clean = append(clean, s)
		}
		if len(clean) > 0 {
			out[k] = clean
		}
	}

	encoded := out.Encode()
	if len(encoded) > 800 {
		return encoded[:800] + "…"
	}
	return encoded
}

// TraceInjectionMiddleware 确保每个请求都有 trace_id（从 header 继承或新建）并注入 context
func TraceInjectionMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID := strings.TrimSpace(c.GetHeader("X-Trace-Id"))
		if traceID == "" {
			traceID = strings.TrimSpace(c.GetHeader("X-Trace-ID"))
		}
		if traceID == "" {
			traceID = uuid.NewString()
		}
		requestID := strings.TrimSpace(c.GetHeader("X-Request-Id"))
		if requestID == "" {
			requestID = strings.TrimSpace(c.GetHeader("X-Request-ID"))
		}
		if requestID == "" {
			requestID = uuid.NewString()
		}
		// 将 trace_id 放进 context 便于以后取
		ctx := c.Request.Context()
		ctx = contextWithTraceID(ctx, traceID)
		ctx = context.WithValue(ctx, requestIDContextKey, requestID)
		c.Request = c.Request.WithContext(ctx)
		c.Set("trace_id", traceID)
		c.Set("request_id", requestID)
		c.Writer.Header().Set("X-Trace-Id", traceID)
		c.Writer.Header().Set("X-Trace-ID", traceID)
		c.Writer.Header().Set("X-Request-Id", requestID)
		c.Writer.Header().Set("X-Request-ID", requestID)
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

func extractPluginIDFromPath(path string) string {
	path = strings.TrimSpace(path)
	if !strings.HasPrefix(path, "/_p/") {
		return ""
	}
	rest := strings.TrimPrefix(path, "/_p/")
	if rest == "" {
		return ""
	}
	if idx := strings.IndexByte(rest, '/'); idx > 0 {
		return rest[:idx]
	}
	return ""
}
