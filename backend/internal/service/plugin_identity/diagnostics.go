package plugin_identity

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"go.uber.org/zap"
)

// rejection 的诊断字段只进入服务端日志，保持公开错误码、消息和响应结构不变。
type rejection struct {
	err    error
	reason string
	fields []zap.Field
}

func (e *rejection) Error() string { return e.err.Error() }
func (e *rejection) Unwrap() error { return e.err }

func rejected(status int, code, reason string, fields ...zap.Field) error {
	return explainRejection(failure(status, code), reason, fields...)
}

func explainRejection(err error, reason string, fields ...zap.Field) error {
	var existing *rejection
	if err == nil || errors.As(err, &existing) {
		return err
	}
	return &rejection{err: err, reason: reason, fields: fields}
}

func issueProof(in IssueInput) proof {
	c := in.Claims
	p := proof{Version: 1, PluginID: in.PluginID, TenantUUID: canonical(c.TenantUUID), UserUUID: canonical(c.UserUUID), MemberUUID: canonical(c.MemberUUID), SessionJTI: c.ID, IsRoot: c.IsRoot, Method: in.Method, Path: in.Path, Transport: in.Transport, RequestID: in.RequestID, TraceID: in.TraceID, RequiredPermission: in.RequiredPermission}
	if c.ExpiresAt != nil {
		p.UserExpiresAt = c.ExpiresAt.Time.UTC()
	}
	return p
}

func (s *Service) logRejected(ctx context.Context, phase string, p proof, err error) {
	if err == nil {
		return
	}
	now := time.Now().UTC()
	if s != nil && s.clock != nil {
		now = s.clock().UTC()
	}
	reason := "identity_check_failed"
	var detail *rejection
	if errors.As(err, &detail) {
		reason = detail.reason
	}
	sessionRef := ""
	if p.SessionJTI != "" {
		sessionRef = hash(p.SessionJTI)
	}
	pluginID := ""
	if len(p.PluginID) <= 128 && pluginPattern.MatchString(p.PluginID) {
		pluginID = p.PluginID
	}
	fields := []zap.Field{
		zap.String("phase", phase), zap.String("reason", reason),
		zap.Int("http_status", dto.StatusCode(err)), zap.String("error_code", dto.CodeOf(err)),
		zap.String("plugin_id", pluginID), zap.String("tenant_uuid", canonical(p.TenantUUID)),
		zap.String("user_uuid", canonical(p.UserUUID)), zap.String("member_uuid", canonical(p.MemberUUID)),
		zap.String("session_ref", sessionRef), zap.Bool("is_root", p.IsRoot),
		zap.String("source_request_id", diagnosticID(p.RequestID)), zap.String("source_trace_id", diagnosticID(p.TraceID)),
		zap.String("trace_id", diagnosticID(reqctx.GetTraceID(ctx))),
		zap.String("correlation_id", diagnosticID(audit.CorrelationIDFromContext(ctx))),
		zap.String("method", diagnosticID(p.Method)), zap.String("transport", diagnosticID(p.Transport)),
		zap.String("path", diagnosticPath(p.Path)), zap.String("checked_at", now.Format(time.RFC3339Nano)),
	}
	if !p.UserExpiresAt.IsZero() {
		fields = append(fields, zap.String("user_access_expires_at", p.UserExpiresAt.UTC().Format(time.RFC3339Nano)))
	}
	if !p.ExpiresAt.IsZero() {
		fields = append(fields, zap.String("proof_expires_at", p.ExpiresAt.UTC().Format(time.RFC3339Nano)))
	}
	if detail != nil {
		fields = append(fields, detail.fields...)
	}
	write := logger.Warn
	if s != nil && s.rejectionLogger != nil {
		write = s.rejectionLogger
	}
	write(ctx, "iam_request_identity_rejected", fields...)
}

// 关联标识只允许有限字符，路径不保留 query/hash；不记录 Token、票据或依赖原始错误。
func diagnosticID(value string) string {
	if len(value) > 128 {
		return ""
	}
	for _, c := range value {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("._:-", c)) {
			return ""
		}
	}
	return value
}

func diagnosticPath(value string) string {
	value, _, _ = strings.Cut(value, "?")
	value, _, _ = strings.Cut(value, "#")
	if !strings.HasPrefix(value, "/") || strings.ContainsAny(value, "\r\n") {
		return ""
	}
	if len(value) > 512 {
		value = value[:512]
	}
	return value
}
