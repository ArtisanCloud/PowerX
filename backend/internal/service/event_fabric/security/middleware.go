package security

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	pxlog "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

var (
	// ErrTLSRequired 表示当前请求必须使用 TLS。
	ErrTLSRequired = errors.New("event_fabric: tls required")
	// ErrSignatureMissing 表示请求缺少签名或时间戳。
	ErrSignatureMissing = errors.New("event_fabric: signature missing")
	// ErrSignatureInvalid 表示签名无效。
	ErrSignatureInvalid = errors.New("event_fabric: signature invalid")
	// ErrTimestampExpired 表示时间戳超出允许的偏移。
	ErrTimestampExpired = errors.New("event_fabric: signature timestamp expired")
	// ErrSandboxViolation 表示请求违反了沙箱策略。
	ErrSandboxViolation = errors.New("event_fabric: sandbox violation")
)

// Config 定义事件骨干安全校验的参数。
type Config struct {
	RequireTLS           bool
	SignatureSecret      string
	SignatureHeader      string
	TimestampHeader      string
	SignatureKeyID       string
	AllowedClockSkew     time.Duration
	ProtectedGRPCService string
	Sandbox              SandboxConfig
}

// SandboxConfig 描述 Agent/插件沙箱访问策略。
type SandboxConfig struct {
	Enforce              bool
	AllowedOutboundHosts []string
	BlockedHTTPPaths     []string
	BlockedGRPCMethods   []string
	ForbiddenHeaders     []string
}

// Violation 描述一次沙箱违规事件。
type Violation struct {
	Type      string
	Method    string
	Path      string
	Host      string
	Detail    string
	Timestamp time.Time
}

// ViolationReporter 用于记录或告警沙箱违规事件。
type ViolationReporter interface {
	Report(ctx context.Context, violation Violation)
}

type multiReporter []ViolationReporter

// NewMultiReporter 聚合多个 Reporter。
func NewMultiReporter(reporters ...ViolationReporter) ViolationReporter {
	var list multiReporter
	for _, r := range reporters {
		if r != nil {
			list = append(list, r)
		}
	}
	if len(list) == 0 {
		return nil
	}
	return list
}

func (m multiReporter) Report(ctx context.Context, violation Violation) {
	for _, reporter := range m {
		reporter.Report(ctx, violation)
	}
}

type loggerReporter struct {
	logger *pxlog.Logger
}

// NewLoggerReporter 使用日志记录沙箱违规。
func NewLoggerReporter(logger *pxlog.Logger) ViolationReporter {
	if logger == nil {
		logger = pxlog.GetGlobalLogger()
	}
	return loggerReporter{logger: logger}
}

func (l loggerReporter) Report(ctx context.Context, violation Violation) {
	if l.logger == nil {
		return
	}
	l.logger.WarnF(ctx, "[security.sandbox] violation type=%s method=%s path=%s host=%s detail=%s", violation.Type, violation.Method, violation.Path, violation.Host, violation.Detail)
}

// Verifier 提供 HTTP/GPRC 安全校验。
type Verifier struct {
	cfg                  Config
	signatureHeaderLower string
	timestampHeaderLower string
	sandbox              SandboxConfig
	reporter             ViolationReporter
}

// NewVerifier 构建安全校验器。
func NewVerifier(cfg Config) *Verifier {
	if cfg.SignatureHeader == "" {
		cfg.SignatureHeader = "X-PowerX-Signature"
	}
	if cfg.TimestampHeader == "" {
		cfg.TimestampHeader = "X-PowerX-Timestamp"
	}
	if cfg.AllowedClockSkew <= 0 {
		cfg.AllowedClockSkew = 5 * time.Minute
	}
	if cfg.ProtectedGRPCService == "" {
		cfg.ProtectedGRPCService = "/corex.event_fabric.v1."
	}
	sandbox := normalizeSandbox(cfg.Sandbox)
	return &Verifier{
		cfg:                  cfg,
		signatureHeaderLower: strings.ToLower(cfg.SignatureHeader),
		timestampHeaderLower: strings.ToLower(cfg.TimestampHeader),
		sandbox:              sandbox,
		reporter:             NewLoggerReporter(pxlog.GetGlobalLogger()),
	}
}

// SetViolationReporter 注入沙箱违规报告器。
func (v *Verifier) SetViolationReporter(reporter ViolationReporter) {
	if v == nil || reporter == nil {
		return
	}
	if v.reporter == nil {
		v.reporter = reporter
		return
	}
	v.reporter = NewMultiReporter(v.reporter, reporter)
}

// VerifyHTTPRequest 对 HTTP 请求执行安全校验。
func (v *Verifier) VerifyHTTPRequest(r *http.Request) error {
	if v == nil {
		return nil
	}
	if v.cfg.RequireTLS && r.TLS == nil {
		return ErrTLSRequired
	}
	if strings.TrimSpace(v.cfg.SignatureSecret) == "" {
		return nil
	}

	sig := strings.TrimSpace(r.Header.Get(v.cfg.SignatureHeader))
	ts := strings.TrimSpace(r.Header.Get(v.cfg.TimestampHeader))
	if sig == "" || ts == "" {
		return ErrSignatureMissing
	}
	if err := v.verifyTimestamp(ts); err != nil {
		return err
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("read request body: %w", err)
	}
	r.Body = io.NopCloser(bytes.NewReader(body))

	canonical := strings.Join([]string{
		ts,
		strings.ToUpper(r.Method),
		r.URL.RequestURI(),
		string(body),
	}, "\n")
	if err := v.verifySignature(sig, canonical); err != nil {
		return err
	}
	return v.enforceHTTPSandbox(r)
}

// GinMiddleware 返回 Gin 中间件。
func (v *Verifier) GinMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := v.VerifyHTTPRequest(c.Request); err != nil {
			statusCode := http.StatusUnauthorized
			switch err {
			case ErrTLSRequired:
				statusCode = http.StatusUpgradeRequired
			case ErrSignatureMissing:
				statusCode = http.StatusUnauthorized
			case ErrSignatureInvalid:
				statusCode = http.StatusForbidden
			case ErrTimestampExpired:
				statusCode = http.StatusUnauthorized
			case ErrSandboxViolation:
				statusCode = http.StatusForbidden
			default:
				statusCode = http.StatusForbidden
			}
			c.AbortWithStatusJSON(statusCode, gin.H{
				"code":    statusCode,
				"message": "event fabric request rejected",
				"error":   err.Error(),
			})
			return
		}
		c.Next()
	}
}

// UnaryServerInterceptor 返回 gRPC Unary 拦截器。
func (v *Verifier) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if v == nil || !v.shouldProtect(info.FullMethod) {
			return handler(ctx, req)
		}
		if err := v.verifyGRPC(ctx, info.FullMethod); err != nil {
			return nil, toStatusErr(err)
		}
		return handler(ctx, req)
	}
}

// StreamServerInterceptor 返回 gRPC Stream 拦截器。
func (v *Verifier) StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if v == nil || !v.shouldProtect(info.FullMethod) {
			return handler(srv, stream)
		}
		if err := v.verifyGRPC(stream.Context(), info.FullMethod); err != nil {
			return toStatusErr(err)
		}
		return handler(srv, stream)
	}
}

func (v *Verifier) shouldProtect(fullMethod string) bool {
	return strings.Contains(fullMethod, v.cfg.ProtectedGRPCService)
}

func (v *Verifier) verifyGRPC(ctx context.Context, fullMethod string) error {
	if v.cfg.RequireTLS {
		if p, ok := peer.FromContext(ctx); !ok || p == nil || p.AuthInfo == nil {
			return ErrTLSRequired
		}
	}
	if strings.TrimSpace(v.cfg.SignatureSecret) == "" {
		return nil
	}

	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ErrSignatureMissing
	}
	sig := first(md[v.signatureHeaderLower])
	ts := first(md[v.timestampHeaderLower])
	if sig == "" || ts == "" {
		return ErrSignatureMissing
	}
	if err := v.verifyTimestamp(ts); err != nil {
		return err
	}
	canonical := strings.Join([]string{ts, fullMethod}, "\n")
	if err := v.verifySignature(sig, canonical); err != nil {
		return err
	}
	return v.enforceGRPCSandbox(ctx, fullMethod)
}

func (v *Verifier) verifyTimestamp(raw string) error {
	ts, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return ErrTimestampExpired
	}
	now := time.Now().UTC()
	skew := now.Sub(ts)
	if skew < 0 {
		skew = -skew
	}
	if skew > v.cfg.AllowedClockSkew {
		return ErrTimestampExpired
	}
	return nil
}

func (v *Verifier) verifySignature(headerValue, message string) error {
	parts := strings.SplitN(headerValue, ":", 2)
	if len(parts) != 2 {
		return ErrSignatureInvalid
	}
	keyID := strings.TrimSpace(parts[0])
	if cfgKey := strings.TrimSpace(v.cfg.SignatureKeyID); cfgKey != "" && keyID != cfgKey {
		return ErrSignatureInvalid
	}
	rawSig := strings.TrimSpace(parts[1])
	expected := computeHMAC(v.cfg.SignatureSecret, message)
	provided, err := hex.DecodeString(rawSig)
	if err != nil {
		return ErrSignatureInvalid
	}
	if !hmac.Equal(expected, provided) {
		return ErrSignatureInvalid
	}
	return nil
}

func computeHMAC(secret, message string) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	return mac.Sum(nil)
}

func first(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[0])
}

func toStatusErr(err error) error {
	switch err {
	case ErrTLSRequired:
		return status.Error(codes.PermissionDenied, err.Error())
	case ErrSignatureMissing:
		return status.Error(codes.Unauthenticated, err.Error())
	case ErrSignatureInvalid:
		return status.Error(codes.PermissionDenied, err.Error())
	case ErrTimestampExpired:
		return status.Error(codes.Unauthenticated, err.Error())
	case ErrSandboxViolation:
		return status.Error(codes.PermissionDenied, err.Error())
	default:
		return status.Error(codes.PermissionDenied, err.Error())
	}
}

func (v *Verifier) enforceHTTPSandbox(r *http.Request) error {
	if v == nil || !v.sandbox.Enforce {
		return nil
	}
	host := strings.ToLower(strings.Split(r.Host, ":")[0])
	if len(v.sandbox.AllowedOutboundHosts) > 0 && !hostMatches(host, v.sandbox.AllowedOutboundHosts) {
		v.reportViolation(r.Context(), Violation{
			Type:   "http_host",
			Method: r.Method,
			Path:   r.URL.Path,
			Host:   host,
			Detail: "host not allowed",
		})
		return ErrSandboxViolation
	}
	path := r.URL.Path
	if path == "" {
		path = "/"
	}
	if hasPrefix(path, v.sandbox.BlockedHTTPPaths) {
		v.reportViolation(r.Context(), Violation{
			Type:   "http_path",
			Method: r.Method,
			Path:   path,
			Host:   host,
			Detail: "path blocked",
		})
		return ErrSandboxViolation
	}
	for _, header := range v.sandbox.ForbiddenHeaders {
		if value := strings.TrimSpace(r.Header.Get(header)); value != "" {
			v.reportViolation(r.Context(), Violation{
				Type:   "http_header",
				Method: r.Method,
				Path:   path,
				Host:   host,
				Detail: fmt.Sprintf("header %s not allowed", header),
			})
			return ErrSandboxViolation
		}
	}
	return nil
}

func (v *Verifier) enforceGRPCSandbox(ctx context.Context, method string) error {
	if v == nil || !v.sandbox.Enforce {
		return nil
	}
	if method == "" {
		return nil
	}
	if hasPrefix(method, v.sandbox.BlockedGRPCMethods) {
		v.reportViolation(ctx, Violation{
			Type:   "grpc_method",
			Method: method,
			Detail: "grpc method blocked",
		})
		return ErrSandboxViolation
	}
	return nil
}

func (v *Verifier) reportViolation(ctx context.Context, violation Violation) {
	if v == nil || v.reporter == nil {
		return
	}
	if violation.Timestamp.IsZero() {
		violation.Timestamp = time.Now().UTC()
	}
	v.reporter.Report(ctx, violation)
}

func normalizeSandbox(cfg SandboxConfig) SandboxConfig {
	result := SandboxConfig{Enforce: cfg.Enforce}
	for _, host := range cfg.AllowedOutboundHosts {
		host = strings.ToLower(strings.TrimSpace(host))
		if host != "" {
			result.AllowedOutboundHosts = append(result.AllowedOutboundHosts, host)
		}
	}
	for _, path := range cfg.BlockedHTTPPaths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		result.BlockedHTTPPaths = append(result.BlockedHTTPPaths, path)
	}
	for _, method := range cfg.BlockedGRPCMethods {
		method = strings.TrimSpace(method)
		if method != "" {
			result.BlockedGRPCMethods = append(result.BlockedGRPCMethods, method)
		}
	}
	for _, header := range cfg.ForbiddenHeaders {
		header = http.CanonicalHeaderKey(strings.TrimSpace(header))
		if header != "" {
			result.ForbiddenHeaders = append(result.ForbiddenHeaders, header)
		}
	}
	return result
}

func hostMatches(host string, patterns []string) bool {
	for _, pattern := range patterns {
		if pattern == "" {
			continue
		}
		if strings.HasPrefix(pattern, "*.") {
			suffix := strings.TrimPrefix(pattern, "*")
			if strings.HasSuffix(host, suffix) {
				return true
			}
		} else if host == pattern {
			return true
		}
	}
	return false
}

func hasPrefix(value string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if prefix != "" && strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}
