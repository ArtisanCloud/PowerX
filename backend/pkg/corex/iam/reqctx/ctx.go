package reqctx

import (
	"context"
	"errors"
	"fmt"

	"github.com/ArtisanCloud/PowerX/pkg/corex/env"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type ctxKey string

// —— 中间件常用键（你原有的一组）——
const (
	TenantIDKey    ctxKey = "tenant_id"
	TenantUUIDKey  ctxKey = "tenant_uuid"
	SubjectKey     ctxKey = "subject"
	ScopeKey       ctxKey = "scope"
	AudienceKey    ctxKey = "audience"
	PlatformKey    ctxKey = "platform"
	TraceIDKey     ctxKey = "trace_id"
	JWTClaimsKey   ctxKey = "jwt_claims"
	RequestPathKey ctxKey = "request_path"

	UserIDKey   ctxKey = "auth.user_id"
	MemberIDKey ctxKey = "auth.member_id"
	IsRootKey   ctxKey = "auth.is_root"
)

// CoreXClaims 统一的业务 Claims
type CoreXClaims struct {
	Env        string   `json:"env,omitempty"`
	Envs       []string `json:"envs,omitempty"`
	TenantUUID string   `json:"tid"`
	TenantID   uint64   `json:"tid_n"`
	MemberUUID string   `json:"mid"` // 也会作为 Subject
	MemberID   uint64   `json:"mid_n"`
	UserUUID   string   `json:"uid"`
	UserID     uint64   `json:"uid_n"`
	Email      string   `json:"email,omitempty"`
	Phone      string   `json:"phone,omitempty"`
	IsRoot     bool     `json:"is_root"`
	Roles      []string `json:"roles,omitempty"`
	Platforms  []string `json:"plats,omitempty"`
	Scope      string   `json:"scope"`

	jwt.RegisteredClaims
}

// —— 统一键（建议业务层优先使用这一组）——
const (
	KeyClaims          ctxKey = "corex.claims"
	KeyTenantID        ctxKey = "corex.tenant_id"
	KeyTenantUUID      ctxKey = "corex.tenant_uuid"
	KeyTenantUUIDValue ctxKey = "corex.tenant_uuid_value"
	KeyUserID          ctxKey = "corex.user_id"
	KeyUserUUID        ctxKey = "corex.user_uuid"
	KeyMemberID        ctxKey = "corex.member_id"
	KeyMemberUUID      ctxKey = "corex.member_uuid"
	KeyIsRoot          ctxKey = "corex.is_root"
	KeySubject         ctxKey = "corex.subject"
	KeyAudience        ctxKey = "corex.audience"
	KeyPlatform        ctxKey = "corex.platform"
	KeyTraceID         ctxKey = "corex.trace_id"
	KeyRequestPath     ctxKey = "corex.request_path"

	KeyEnv  ctxKey = "corex.env"
	KeyEnvs ctxKey = "corex.envs"
)

var (
	ErrTenantMissing     = errors.New("tenant_id missing")
	ErrTenantUUIDMissing = errors.New("tenant_uuid missing")
	ErrEnvMissing        = errors.New("env missing")
	ErrClaimsMissing     = errors.New("jwt claims missing")
)

/* ===================== Setters（中间件使用） ===================== */

func WithClaims(ctx context.Context, c *CoreXClaims) context.Context {
	return context.WithValue(ctx, KeyClaims, c)
}
func WithTenantID(ctx context.Context, v uint64) context.Context {
	return context.WithValue(ctx, KeyTenantID, v)
}
func WithTenantUUID(ctx context.Context, v string) context.Context {
	return context.WithValue(ctx, KeyTenantUUID, v)
}
func WithTenantUUIDValue(ctx context.Context, v uuid.UUID) context.Context {
	if v == uuid.Nil {
		return ctx
	}
	return context.WithValue(ctx, KeyTenantUUIDValue, v)
}
func WithUserID(ctx context.Context, v uint64) context.Context {
	return context.WithValue(ctx, KeyUserID, v)
}
func WithUserUUID(ctx context.Context, v string) context.Context {
	return context.WithValue(ctx, KeyUserUUID, v)
}
func WithMemberID(ctx context.Context, v uint64) context.Context {
	return context.WithValue(ctx, KeyMemberID, v)
}
func WithMemberUUID(ctx context.Context, v string) context.Context {
	return context.WithValue(ctx, KeyMemberUUID, v)
}
func WithIsRoot(ctx context.Context, v bool) context.Context {
	return context.WithValue(ctx, KeyIsRoot, v)
}
func WithSubject(ctx context.Context, v string) context.Context {
	return context.WithValue(ctx, KeySubject, v)
}
func WithAudience(ctx context.Context, v string) context.Context {
	return context.WithValue(ctx, KeyAudience, v)
}
func WithPlatform(ctx context.Context, v string) context.Context {
	return context.WithValue(ctx, KeyPlatform, v)
}
func WithTraceID(ctx context.Context, v string) context.Context {
	return context.WithValue(ctx, KeyTraceID, v)
}
func WithRequestPath(ctx context.Context, v string) context.Context {
	return context.WithValue(ctx, KeyRequestPath, v)
}
func WithEnv(ctx context.Context, e string) context.Context {
	return context.WithValue(ctx, KeyEnv, env.Canonicalize(e))
}
func WithEnvs(ctx context.Context, es []string) context.Context {
	if len(es) == 0 {
		return ctx
	}
	out := make([]string, 0, len(es))
	for _, s := range es {
		if s = env.Canonicalize(s); s != "" {
			out = append(out, s)
		}
	}
	return context.WithValue(ctx, KeyEnvs, out)
}

/* ===================== Getters（业务/Repo） ===================== */

func GetClaims(ctx context.Context) *CoreXClaims {
	if v, ok := ctx.Value(KeyClaims).(*CoreXClaims); ok && v != nil {
		return v
	}
	// 兼容：有些地方可能直接把 claims 放到了 JWTClaimsKey
	if v, ok := ctx.Value(JWTClaimsKey).(*CoreXClaims); ok && v != nil {
		return v
	}
	return nil
}

func GetTenantID(ctx context.Context) uint64 {
	// 统一键优先
	if v, ok := ctx.Value(KeyTenantID).(uint64); ok && v > 0 {
		return v
	}
	// 兼容中间件键
	if v, ok := ctx.Value(TenantIDKey).(uint64); ok && v > 0 {
		return v
	}
	// 兜底 claims
	if c := GetClaims(ctx); c != nil && c.TenantID > 0 {
		return c.TenantID
	}
	return 0
}
func RequireTenantID(ctx context.Context) (uint64, error) {
	if id := GetTenantID(ctx); id > 0 {
		return id, nil
	}
	return 0, ErrTenantMissing
}

func GetTenantUUID(ctx context.Context) string {
	// 统一键
	if v, ok := ctx.Value(KeyTenantUUID).(string); ok && v != "" {
		return v
	}
	// 兼容中间件键
	if v, ok := ctx.Value(TenantUUIDKey).(string); ok && v != "" {
		return v
	}
	// 兜底 claims
	if c := GetClaims(ctx); c != nil && c.TenantUUID != "" {
		return c.TenantUUID
	}
	return ""
}

func RequireTenantUUID(ctx context.Context) (string, error) {
	if uuid := GetTenantUUID(ctx); uuid != "" {
		return uuid, nil
	}
	return "", ErrTenantUUIDMissing
}

func TenantUUIDValue(ctx context.Context) uuid.UUID {
	if v, ok := ctx.Value(KeyTenantUUIDValue).(uuid.UUID); ok && v != uuid.Nil {
		return v
	}
	if raw := GetTenantUUID(ctx); raw != "" {
		if parsed, err := uuid.Parse(raw); err == nil {
			return parsed
		}
	}
	return uuid.Nil
}

func RequireTenantUUIDValue(ctx context.Context) (uuid.UUID, error) {
	if v, ok := ctx.Value(KeyTenantUUIDValue).(uuid.UUID); ok && v != uuid.Nil {
		return v, nil
	}
	raw := GetTenantUUID(ctx)
	if raw == "" {
		return uuid.Nil, ErrTenantUUIDMissing
	}
	parsed, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid tenant uuid in context: %w", err)
	}
	return parsed, nil
}

func GetUserID(ctx context.Context) uint64 {
	if v, ok := ctx.Value(KeyUserID).(uint64); ok && v > 0 {
		return v
	}
	if v, ok := ctx.Value(UserIDKey).(uint64); ok && v > 0 {
		return v
	}
	if c := GetClaims(ctx); c != nil && c.UserID > 0 {
		return c.UserID
	}
	return 0
}

func GetUserUUID(ctx context.Context) string {
	if v, ok := ctx.Value(KeyUserUUID).(string); ok && v != "" {
		return v
	}
	if c := GetClaims(ctx); c != nil && c.UserUUID != "" {
		return c.UserUUID
	}
	return ""
}

func GetMemberID(ctx context.Context) uint64 {
	if v, ok := ctx.Value(KeyMemberID).(uint64); ok && v > 0 {
		return v
	}
	if v, ok := ctx.Value(MemberIDKey).(uint64); ok && v > 0 {
		return v
	}
	if c := GetClaims(ctx); c != nil && c.MemberID > 0 {
		return c.MemberID
	}
	return 0
}

func GetMemberUUID(ctx context.Context) string {
	if v, ok := ctx.Value(KeyMemberUUID).(string); ok && v != "" {
		return v
	}
	if c := GetClaims(ctx); c != nil && c.MemberUUID != "" {
		return c.MemberUUID
	}
	return ""
}

func IsRoot(ctx context.Context) bool {
	if v, ok := ctx.Value(KeyIsRoot).(bool); ok {
		return v
	}
	if v, ok := ctx.Value(IsRootKey).(bool); ok {
		return v
	}
	if c := GetClaims(ctx); c != nil {
		return c.IsRoot
	}
	return false
}

func GetSubject(ctx context.Context) string {
	if v, ok := ctx.Value(KeySubject).(string); ok && v != "" {
		return v
	}
	if v, ok := ctx.Value(SubjectKey).(string); ok && v != "" {
		return v
	}
	if c := GetClaims(ctx); c != nil && c.MemberUUID != "" {
		return c.MemberUUID
	}
	if c := GetClaims(ctx); c != nil && c.UserUUID != "" {
		return c.UserUUID
	}
	return ""
}

func GetScope(ctx context.Context) string {
	if v, ok := ctx.Value(ScopeKey).(string); ok && v != "" {
		return v
	}
	if c := GetClaims(ctx); c != nil && c.Scope != "" {
		return c.Scope
	}
	return ""
}

func GetAudience(ctx context.Context) string {
	if v, ok := ctx.Value(KeyAudience).(string); ok && v != "" {
		return v
	}
	if v, ok := ctx.Value(AudienceKey).(string); ok && v != "" {
		return v
	}
	// claims.Audience 是 []string；这里做个简单兜底
	if c := GetClaims(ctx); c != nil && len(c.Audience) > 0 {
		return c.Audience[0]
	}
	return ""
}

func GetPlatform(ctx context.Context) string {
	if v, ok := ctx.Value(KeyPlatform).(string); ok && v != "" {
		return v
	}
	if v, ok := ctx.Value(PlatformKey).(string); ok && v != "" {
		return v
	}
	// claims.Platforms 是 []string；取第一个
	if c := GetClaims(ctx); c != nil && len(c.Platforms) > 0 {
		return c.Platforms[0]
	}
	return ""
}

func GetTraceID(ctx context.Context) string {
	if v, ok := ctx.Value(KeyTraceID).(string); ok && v != "" {
		return v
	}
	if v, ok := ctx.Value(TraceIDKey).(string); ok && v != "" {
		return v
	}
	return ""
}

func GetRequestPath(ctx context.Context) string {
	if v, ok := ctx.Value(KeyRequestPath).(string); ok && v != "" {
		return v
	}
	if v, ok := ctx.Value(RequestPathKey).(string); ok && v != "" {
		return v
	}
	return ""
}

func GetEnv(ctx context.Context) string {
	if v, ok := ctx.Value(KeyEnv).(string); ok && v != "" {
		return env.Canonicalize(v)
	}
	if c := GetClaims(ctx); c != nil && c.Env != "" {
		return env.Canonicalize(c.Env)
	}
	return ""
}
func RequireEnv(ctx context.Context) (string, error) {
	if e := GetEnv(ctx); e != "" {
		return e, nil
	}
	return "", ErrEnvMissing
}

func GetEnvWhitelist(ctx context.Context) []string {
	if v := ctx.Value(KeyEnvs); v != nil {
		if ss, ok := v.([]string); ok && len(ss) > 0 {
			out := make([]string, 0, len(ss))
			for _, s := range ss {
				if s = env.Canonicalize(s); s != "" {
					out = append(out, s)
				}
			}
			return out
		}
	}
	if c := GetClaims(ctx); c != nil && len(c.Envs) > 0 {
		out := make([]string, 0, len(c.Envs))
		for _, s := range c.Envs {
			if s = env.Canonicalize(s); s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

/* ===================== CB 示例 ===================== */

func RootOnlyCB() func(ctx context.Context, claims *CoreXClaims) error {
	return func(ctx context.Context, claims *CoreXClaims) error {
		if claims == nil || !claims.IsRoot {
			return fmt.Errorf("root only")
		}
		return nil
	}
}
