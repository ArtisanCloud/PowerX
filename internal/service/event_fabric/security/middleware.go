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
}

// Verifier 提供 HTTP/GPRC 安全校验。
type Verifier struct {
	cfg                  Config
	signatureHeaderLower string
	timestampHeaderLower string
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
	return &Verifier{
		cfg:                  cfg,
		signatureHeaderLower: strings.ToLower(cfg.SignatureHeader),
		timestampHeaderLower: strings.ToLower(cfg.TimestampHeader),
	}
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
	return v.verifySignature(sig, canonical)
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
	return v.verifySignature(sig, canonical)
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
	default:
		return status.Error(codes.PermissionDenied, err.Error())
	}
}
