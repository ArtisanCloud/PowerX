// pkg/auth/middleware.go
package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/ArtisanCloud/PowerX/pkg/auth"
	"github.com/ArtisanCloud/PowerX/pkg/cache"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	pxlog "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func KUser(uid uint64) string    { return "auth:user:" + strconv.FormatUint(uid, 10) }
func KMember(mid uint64) string  { return "auth:member:" + strconv.FormatUint(mid, 10) }
func KTenant(tid uint64) string  { return "auth:tenant:" + strconv.FormatUint(tid, 10) }
func KRevoked(jti string) string { return "auth:revoked:" + jti }

// JwtMiddleware 统一的 JWT 校验中间件
// - issuer/audiences：与签发保持一致（从配置传入）
// - requiredScopes：允许的 scope（例如只允许 "access"），传空则不限制；支持 "*" 代表任意
// - cb：通过则回调做额外校验（比如租户冻结、风控等）；返回 error 即拒绝
func JwtMiddleware(
	secret []byte,
	issuer string,
	audiences []string,
	requiredScopes []string,
	cb func(ctx context.Context, claims *reqctx.CoreXClaims) error,
	opts ...JwtOption,
) gin.HandlerFunc {
	cfg := jwtMiddlewareConfig{
		headerPolicy: TenantHeaderPolicy{
			RequireUUID: true,
		},
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	return func(c *gin.Context) {
		// 1) Authorization: Bearer <token>
		authz := c.GetHeader("Authorization")
		if authz == "" || !strings.HasPrefix(strings.ToLower(authz), "bearer ") {
			pxlog.WarnF(
				c.Request.Context(),
				"[auth.jwt] unauthorized stage=missing_bearer method=%s path=%s trace_id=%s reason=%q issuer=%s audiences=%v",
				c.Request.Method,
				c.Request.URL.Path,
				reqctx.GetTraceID(c.Request.Context()),
				"missing or invalid Authorization header",
				issuer,
				audiences,
			)
			if !abortIAMMemberDirectoryAuthError(c, http.StatusUnauthorized, "IAM_UNAUTHORIZED") {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing or invalid Authorization header"})
			}
			return
		}
		tokenString := strings.TrimSpace(authz[len("Bearer "):])
		if tokenString == "" {
			pxlog.WarnF(
				c.Request.Context(),
				"[auth.jwt] unauthorized stage=empty_bearer method=%s path=%s trace_id=%s reason=%q issuer=%s audiences=%v",
				c.Request.Method,
				c.Request.URL.Path,
				reqctx.GetTraceID(c.Request.Context()),
				"invalid bearer token",
				issuer,
				audiences,
			)
			if !abortIAMMemberDirectoryAuthError(c, http.StatusUnauthorized, "IAM_UNAUTHORIZED") {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid bearer token"})
			}
			return
		}

		reqCtx := reqctx.WithRequestPath(c.Request.Context(), c.Request.URL.Path)
		reqCtx = reqctx.WithRequestMethod(reqCtx, c.Request.Method)

		// A. 解析 + 标准校验（Issuer / Audience / exp / nbf / iat / 签名）
		// 注意：ParseAndValidate 需返回 *reqctx.CoreXClaims（见 pkg/auth/jwt.go）
		claims, err := parseWithConfiguredChecks(tokenString, secret, issuer, audiences, cfg.extraTokenChecks)
		if err != nil {
			pxlog.WarnF(
				c.Request.Context(),
				"[auth.jwt] unauthorized stage=parse_validate_failed method=%s path=%s trace_id=%s reason=%q issuer=%s audiences=%v",
				c.Request.Method,
				c.Request.URL.Path,
				reqctx.GetTraceID(c.Request.Context()),
				err.Error(),
				issuer,
				audiences,
			)
			if !abortIAMMemberDirectoryAuthError(c, http.StatusUnauthorized, "IAM_UNAUTHORIZED") {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			}
			return
		}

		// B. scope 检查
		if len(requiredScopes) > 0 && !scopeAllowed(claims.Scope, requiredScopes) {
			if !abortIAMMemberDirectoryAuthError(c, http.StatusForbidden, "IAM_FORBIDDEN") {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "scope not allowed"})
			}
			return
		}

		// C. 撤销/强制下线（仅 access 携带 jti 时检查）
		authCache := cache.GetCache()
		if authCache != nil && claims.ID != "" {
			if ok, _ := authCache.Exists(reqCtx, KRevoked(claims.ID)); ok {
				pxlog.WarnF(
					c.Request.Context(),
					"[auth.jwt] unauthorized stage=token_revoked method=%s path=%s trace_id=%s reason=%q issuer=%s audiences=%v",
					c.Request.Method,
					c.Request.URL.Path,
					reqctx.GetTraceID(c.Request.Context()),
					"token revoked",
					issuer,
					audiences,
				)
				if !abortIAMMemberDirectoryAuthError(c, http.StatusUnauthorized, "IAM_UNAUTHORIZED") {
					c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "token revoked"})
				}
				return
			}
		}

		// D. 租户来源策略：仅依据 JWT claims，不接受 query/header/body 注入。
		tenantUUID := strings.TrimSpace(claims.TenantUUID)

		tenantUUID = strings.TrimSpace(tenantUUID)
		tenantID := claims.TenantID
		var tenantUUIDValue uuid.UUID
		if tenantUUID == "" {
			if cfg.headerPolicy.RequireUUID {
				incTenantHeaderReject()
				if !abortIAMMemberDirectoryAuthError(c, http.StatusUnauthorized, "IAM_UNAUTHORIZED") {
					c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "tenant uuid required"})
				}
				return
			}
		} else {
			canonical, err := reqctx.CanonicalTenantUUID(tenantUUID)
			if err != nil {
				incTenantHeaderReject()
				if !abortIAMMemberDirectoryAuthError(c, http.StatusUnauthorized, "IAM_UNAUTHORIZED") {
					c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid tenant uuid"})
				}
				return
			}
			tenantUUID = canonical
			if parsed, err := uuid.Parse(canonical); err == nil {
				tenantUUIDValue = parsed
			}
		}

		// E. 读取缓存快照（命中才做轻量校验；未命中交给 cb 处理）
		var userSnap, memberSnap map[string]any
		if authCache != nil {
			if b, _ := authCache.Get(reqCtx, KUser(claims.UserID)); len(b) > 0 {
				_ = json.Unmarshal(b, &userSnap)
			}
			// 非 Root 才尝试读 member 快照；Root 代理时不需要 member 校验
			if !claims.IsRoot && claims.MemberID > 0 {
				if b, _ := authCache.Get(reqCtx, KMember(claims.MemberID)); len(b) > 0 {
					_ = json.Unmarshal(b, &memberSnap)
				}
			}
		}

		// F. 轻量状态校验（仅在命中缓存时执行）
		if userSnap != nil {
			if st, ok := utils.AsInt16(userSnap["status"]); ok && st != 1 {
				if !abortIAMMemberDirectoryAuthError(c, http.StatusForbidden, "IAM_FORBIDDEN") {
					c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "user disabled"})
				}
				return
			}
		}
		// Root 场景不校验 member（代理租户时可能没有成员记录）
		if !claims.IsRoot && memberSnap != nil {
			if st, ok := utils.AsInt16(memberSnap["status"]); ok && st != 1 {
				if !abortIAMMemberDirectoryAuthError(c, http.StatusForbidden, "IAM_FORBIDDEN") {
					c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "member disabled"})
				}
				return
			}
			if mtid, ok := utils.AsUint64(memberSnap["tenant_id"]); ok && mtid != tenantID {
				if !abortIAMMemberDirectoryAuthError(c, http.StatusForbidden, "IAM_FORBIDDEN") {
					c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "tenant mismatch"})
				}
				return
			}
			if uid, ok := utils.AsUint64(memberSnap["user_id"]); ok && uid != claims.UserID {
				if !abortIAMMemberDirectoryAuthError(c, http.StatusForbidden, "IAM_FORBIDDEN") {
					c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "user mismatch"})
				}
				return
			}
		}
		// G. 注入上下文（统一使用 reqctx.With*，并修正 platform 类型）
		reqCtx = reqctx.WithClaims(reqCtx, claims)
		reqCtx = reqctx.WithTenantUUID(reqCtx, tenantUUID)
		if tenantUUIDValue != uuid.Nil {
			reqCtx = reqctx.WithTenantUUIDValue(reqCtx, tenantUUIDValue)
		}

		// 常用 id / root
		reqCtx = reqctx.WithUserID(reqCtx, claims.UserID)
		reqCtx = reqctx.WithMemberID(reqCtx, claims.MemberID)
		reqCtx = reqctx.WithIsRoot(reqCtx, claims.IsRoot)

		// subject / audience / platform
		subject := strings.TrimSpace(claims.Subject)
		if subject == "" {
			subject = strings.TrimSpace(claims.MemberUUID)
		}
		reqCtx = reqctx.WithSubject(reqCtx, subject)
		if len(claims.Audience) > 0 {
			reqCtx = reqctx.WithAudience(reqCtx, claims.Audience[0])
		}
		plat := ""
		if len(claims.Platforms) > 0 {
			plat = claims.Platforms[0] // 统一写入 string，避免后续读取类型不匹配
		}
		reqCtx = reqctx.WithPlatform(reqCtx, plat)

		// 环境：把 token 里的 env / envs 也写入（下游可直接 reqctx.GetEnv 使用）
		reqCtx = reqctx.WithEnv(reqCtx, claims.Env)
		reqCtx = reqctx.WithEnvs(reqCtx, claims.Envs)

		// TraceID（可选：从 Header 透传）
		if tr := c.GetHeader("X-Trace-Id"); tr != "" {
			reqCtx = reqctx.WithTraceID(reqCtx, tr)
		}

		// 写回 request，并同步到 gin.Context 的 keys
		c.Request = c.Request.WithContext(reqCtx)
		reqctx.CopyCtxToGin(c)
		// 兼容历史：部分 handler 仍通过 "auth_claims" 键读取 claims
		c.Set("auth_claims", *claims)

		// H. 业务回调：缓存 miss 或需要强校验时，cb 回源 DB
		if cb != nil {
			if err := cb(reqCtx, claims); err != nil {
				if !abortIAMMemberDirectoryAuthError(c, http.StatusForbidden, "IAM_FORBIDDEN") {
					c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": err.Error()})
				}
				return
			}
		}

		incTenantUUIDOnlyRequest()
		c.Next()
	}
}

// abortIAMMemberDirectoryAuthError keeps the published IAM delegated-directory
// contract intact when the request is rejected before its route handler runs.
// The global middleware deliberately owns authentication, so this mapping must
// live here rather than in the directory handler.
func abortIAMMemberDirectoryAuthError(c *gin.Context, status int, reasonCode string) bool {
	if !isIAMMemberDirectoryPath(c.Request.URL.Path) {
		return false
	}
	dto.ResponseError(c, status, reasonCode, dto.NewErrorWithCode(status, reasonCode, reasonCode, errors.New(reasonCode)))
	c.Abort()
	return true
}

func isIAMMemberDirectoryPath(path string) bool {
	path = strings.TrimSuffix(strings.TrimSpace(path), "/")
	for _, prefix := range []string{"/api/v1/tenant/iam/members", "/api/tenant/iam/members"} {
		if path == prefix+":batch-get" || path == prefix+":batch-resolve" || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	for _, candidate := range []string{
		"/api/v1/tenant/iam/departments",
		"/api/v1/tenant/iam/roles",
		"/api/v1/tenant/iam/permissions",
		"/api/v1/tenant/iam/authorization:check",
		"/api/tenant/iam/departments",
		"/api/tenant/iam/roles",
		"/api/tenant/iam/permissions",
		"/api/tenant/iam/authorization:check",
	} {
		if strings.EqualFold(strings.TrimSpace(path), candidate) {
			return true
		}
	}
	return false
}

func parseWithConfiguredChecks(tokenString string, secret []byte, issuer string, audiences []string, extra []TokenCheck) (*reqctx.CoreXClaims, error) {
	claims, err := auth.ParseAndValidate(tokenString, secret, issuer, audiences...)
	if err == nil {
		return claims, nil
	}
	firstErr := err
	for _, check := range extra {
		checkIssuer := strings.TrimSpace(check.Issuer)
		if checkIssuer == "" {
			checkIssuer = issuer
		}
		claims, err := auth.ParseAndValidate(tokenString, secret, checkIssuer, check.Audiences...)
		if err == nil {
			return claims, nil
		}
	}
	return nil, firstErr
}

func scopeAllowed(got string, allow []string) bool {
	if got == "" {
		return false
	}
	for _, a := range allow {
		if a == "*" || a == got {
			return true
		}
	}
	return false
}
