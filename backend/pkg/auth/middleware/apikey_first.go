package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	apikeycache "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/apikeycache"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	pxlog "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// APIKeyOrJwtMiddleware 按调用方凭证类型进行鉴权：
// - Authorization: ApiKey <key> -> 仅走 API Key
// - Bearer -> 仅走 JWT
// - 其他 scheme/格式 -> 拒绝请求
func APIKeyOrJwtMiddleware(
	db *gorm.DB,
	secret []byte,
	issuer string,
	audiences []string,
	requiredScopes []string,
	cb func(ctx context.Context, claims *reqctx.CoreXClaims) error,
	opts ...JwtOption,
) gin.HandlerFunc {
	jwtMW := JwtMiddleware(secret, issuer, audiences, requiredScopes, cb, opts...)
	if db == nil {
		return jwtMW
	}
	cfg := jwtMiddlewareConfig{
		headerPolicy: TenantHeaderPolicy{
			RequireUUID: true,
		},
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	return func(c *gin.Context) {
		authz := strings.TrimSpace(c.GetHeader("Authorization"))
		if authz == "" {
			jwtMW(c)
			return
		}

		parts := strings.SplitN(authz, " ", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[1]) == "" {
			if !abortIAMMemberDirectoryAuthError(c, http.StatusUnauthorized, "IAM_UNAUTHORIZED") {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid authorization header"})
			}
			return
		}
		scheme := strings.ToLower(strings.TrimSpace(parts[0]))
		if scheme == "apikey" || scheme == "api-key" {
			raw := strings.TrimSpace(parts[1])
			keyHash := hashAPIKey(raw)
			pxlog.DebugF(
				c.Request.Context(),
				"[auth.apikey] inbound method=%s path=%s key_prefix=%s key_fp=%s",
				c.Request.Method,
				c.Request.URL.Path,
				maskAPIKey(raw),
				shortFingerprint(keyHash),
			)
			ok, err := applyAPIKeyContext(c, db, raw, cfg)
			if err != nil {
				pxlog.WarnF(
					c.Request.Context(),
					"[auth.apikey] rejected path=%s key_fp=%s err=%v",
					c.Request.URL.Path,
					shortFingerprint(keyHash),
					err,
				)
				if !abortIAMMemberDirectoryAuthError(c, http.StatusUnauthorized, "IAM_UNAUTHORIZED") {
					c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "api key unauthorized: " + err.Error()})
				}
				return
			}
			if !ok {
				pxlog.WarnF(
					c.Request.Context(),
					"[auth.apikey] not_found path=%s key_fp=%s",
					c.Request.URL.Path,
					shortFingerprint(keyHash),
				)
				if !abortIAMMemberDirectoryAuthError(c, http.StatusUnauthorized, "IAM_UNAUTHORIZED") {
					c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "api key unauthorized: invalid api key"})
				}
				return
			}
			c.Next()
			return
		}
		if scheme != "bearer" {
			if !abortIAMMemberDirectoryAuthError(c, http.StatusUnauthorized, "IAM_UNAUTHORIZED") {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "unsupported authorization scheme"})
			}
			return
		}
		jwtMW(c)
	}
}

func extractAPIKey(c *gin.Context) string {
	authz := strings.TrimSpace(c.GetHeader("Authorization"))
	if authz == "" {
		return ""
	}
	parts := strings.SplitN(authz, " ", 2)
	if len(parts) != 2 {
		return ""
	}
	scheme := strings.ToLower(strings.TrimSpace(parts[0]))
	if scheme != "apikey" && scheme != "api-key" {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func applyAPIKeyContext(c *gin.Context, db *gorm.DB, apiKey string, cfg jwtMiddlewareConfig) (bool, error) {
	repo := iam.NewAPIKeyRepository(db)
	profileRepo := iam.NewAPIKeyProfileRepository(db)
	nowMs := time.Now().UnixMilli()

	normalizedHash := hashAPIKey(apiKey)
	if snapshot, hit, cacheErr := apikeycache.GetAuthSnapshot(c.Request.Context(), normalizedHash); cacheErr == nil && hit && snapshot != nil {
		return applyCachedAPIKeyContext(c, cfg, snapshot, normalizedHash), nil
	}

	candidates := []string{apiKey}
	hashed := normalizedHash
	if hashed != apiKey {
		candidates = append(candidates, hashed)
	}

	var keyRecord *iamModelAdapter
	for _, candidate := range candidates {
		record, err := repo.FindByHash(c.Request.Context(), candidate)
		if err == nil && record != nil {
			keyRecord = &iamModelAdapter{
				id:         record.ID,
				tenantUUID: strings.TrimSpace(record.TenantUUID),
				profileID:  record.ProfileID,
			}
			break
		}
	}
	if keyRecord == nil {
		return false, nil
	}
	if keyRecord.tenantUUID == "" {
		return false, fmt.Errorf("api key tenant uuid missing")
	}
	if keyRecord.profileID == 0 {
		return false, fmt.Errorf("api key profile missing")
	}
	profile, err := profileRepo.GetById(c.Request.Context(), keyRecord.profileID, nil)
	if err != nil {
		return false, fmt.Errorf("load api key profile failed")
	}
	if profile == nil || profile.Status != 1 {
		return false, fmt.Errorf("api key profile disabled")
	}
	if !strings.EqualFold(strings.TrimSpace(profile.TenantUUID), keyRecord.tenantUUID) {
		return false, fmt.Errorf("api key profile tenant mismatch")
	}
	if cfg.headerPolicy.RequireUUID {
		if _, err := reqctx.CanonicalTenantUUID(keyRecord.tenantUUID); err != nil {
			return false, fmt.Errorf("invalid tenant uuid")
		}
	}
	_ = repo.TouchLastUsed(c.Request.Context(), keyRecord.id, nowMs)
	_ = apikeycache.SetAuthSnapshot(c.Request.Context(), normalizedHash, apikeycache.AuthSnapshot{
		KeyID:      keyRecord.id,
		TenantUUID: keyRecord.tenantUUID,
		ProfileID:  keyRecord.profileID,
	})

	principal := "apikey:" + strconv.FormatUint(keyRecord.id, 10)
	claims := &reqctx.CoreXClaims{
		TenantUUID: keyRecord.tenantUUID,
		MemberUUID: principal,
		MemberID:   keyRecord.profileID,
		IsRoot:     false,
		Roles:      []string{"role_api_key_profile"},
		Platforms:  []string{"api_key"},
		Scope:      "api_key",
	}

	ctx := reqctx.WithRequestPath(c.Request.Context(), c.Request.URL.Path)
	ctx = reqctx.WithRequestMethod(ctx, c.Request.Method)
	ctx = reqctx.WithClaims(ctx, claims)
	ctx = reqctx.WithTenantUUID(ctx, keyRecord.tenantUUID)
	ctx = reqctx.WithMemberID(ctx, keyRecord.profileID)
	ctx = reqctx.WithUserID(ctx, 0)
	ctx = reqctx.WithIsRoot(ctx, false)
	ctx = reqctx.WithSubject(ctx, principal)
	ctx = reqctx.WithAudience(ctx, "api_key")
	ctx = reqctx.WithPlatform(ctx, "api_key")
	if tr := strings.TrimSpace(c.GetHeader("X-Trace-Id")); tr != "" {
		ctx = reqctx.WithTraceID(ctx, tr)
	}

	c.Request = c.Request.WithContext(ctx)
	reqctx.CopyCtxToGin(c)
	c.Set("auth_claims", *claims)
	c.Set("auth_source", "api_key")
	c.Set("auth_api_key_id", keyRecord.id)
	c.Set("auth_api_key_hash", normalizedHash)
	c.Set("auth_api_key_tenant_uuid", keyRecord.tenantUUID)
	incTenantUUIDOnlyRequest()
	return true, nil
}

func applyCachedAPIKeyContext(c *gin.Context, cfg jwtMiddlewareConfig, snapshot *apikeycache.AuthSnapshot, keyHash string) bool {
	if snapshot == nil || strings.TrimSpace(snapshot.TenantUUID) == "" || snapshot.ProfileID == 0 {
		return false
	}
	if cfg.headerPolicy.RequireUUID {
		if _, err := reqctx.CanonicalTenantUUID(snapshot.TenantUUID); err != nil {
			return false
		}
	}
	principal := "apikey:" + strconv.FormatUint(snapshot.KeyID, 10)
	claims := &reqctx.CoreXClaims{
		TenantUUID: snapshot.TenantUUID,
		MemberUUID: principal,
		MemberID:   snapshot.ProfileID,
		IsRoot:     false,
		Roles:      []string{"role_api_key_profile"},
		Platforms:  []string{"api_key"},
		Scope:      "api_key",
	}

	ctx := reqctx.WithRequestPath(c.Request.Context(), c.Request.URL.Path)
	ctx = reqctx.WithRequestMethod(ctx, c.Request.Method)
	ctx = reqctx.WithClaims(ctx, claims)
	ctx = reqctx.WithTenantUUID(ctx, snapshot.TenantUUID)
	ctx = reqctx.WithMemberID(ctx, snapshot.ProfileID)
	ctx = reqctx.WithUserID(ctx, 0)
	ctx = reqctx.WithIsRoot(ctx, false)
	ctx = reqctx.WithSubject(ctx, principal)
	ctx = reqctx.WithAudience(ctx, "api_key")
	ctx = reqctx.WithPlatform(ctx, "api_key")
	if tr := strings.TrimSpace(c.GetHeader("X-Trace-Id")); tr != "" {
		ctx = reqctx.WithTraceID(ctx, tr)
	}

	c.Request = c.Request.WithContext(ctx)
	reqctx.CopyCtxToGin(c)
	c.Set("auth_claims", *claims)
	c.Set("auth_source", "api_key")
	c.Set("auth_api_key_id", snapshot.KeyID)
	c.Set("auth_api_key_hash", keyHash)
	c.Set("auth_api_key_tenant_uuid", snapshot.TenantUUID)
	incTenantUUIDOnlyRequest()
	return true
}

func hashAPIKey(raw string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(raw)))
	return hex.EncodeToString(sum[:])
}

func maskAPIKey(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "<empty>"
	}
	if len(value) <= 12 {
		return value
	}
	return value[:12] + "..."
}

func shortFingerprint(hash string) string {
	value := strings.TrimSpace(hash)
	if len(value) <= 12 {
		return value
	}
	return value[:12]
}

type iamModelAdapter struct {
	id         uint64
	tenantUUID string
	profileID  uint64
}
