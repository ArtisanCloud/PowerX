package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	tenantmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	iamrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	tenantrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"gorm.io/gorm"

	"github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	"github.com/ArtisanCloud/PowerX/pkg/cache"
)

const authSubjectCacheTTL = 60 * time.Second

var authCacheCallbackOnce sync.Once

var stsAllowedCoreCapabilityRoutes = []string{
	"/media/assets",
	"/media/assets/{uuid}",
	"/media/assets/{uuid}/presign",
	"/agents/invoke",
	"/agents/stream/sse",
	"/agents/sessions",
	"/ai/llm/invoke",
	"/ai/llm/stream",
	"/ai/llm/models",
	"/ai/llm/sessions",
	"/ai/llm/sessions/{session_id}/messages",
	"/ai/image/invoke",
	"/ai/video/invoke",
	"/ai/tts/invoke",
	"/ai/embedding/invoke",
}

type userSnapshot struct {
	Status int16 `json:"status"`
}

type memberSnapshot struct {
	Status     int16  `json:"status"`
	UserID     uint64 `json:"user_id"`
	TenantUUID string `json:"tenant_uuid"`
}

type tenantSnapshot struct {
	Status int16 `json:"status"`
}

func buildJWTSubjectValidationCallback(db *gorm.DB) func(ctx context.Context, claims *reqctx.CoreXClaims) error {
	if db == nil {
		return validateSTSRouteOnly
	}
	userRepo := iamrepo.NewUserRepository(db)
	memberRepo := iamrepo.NewMemberRepository(db)
	tenantRepo := tenantrepo.NewTenantRepository(db)

	return func(ctx context.Context, claims *reqctx.CoreXClaims) error {
		if err := validateSTSRouteOnly(ctx, claims); err != nil {
			return err
		}
		if isPowerXAPISTSClaims(claims) {
			return nil
		}
		if claims.UserID == 0 {
			return fmt.Errorf("user id missing")
		}

		currentTenant, err := reqctx.RequireTenantUUID(ctx)
		if err != nil {
			return fmt.Errorf("tenant uuid missing")
		}
		tenantUUID, err := reqctx.CanonicalTenantUUID(currentTenant)
		if err != nil {
			return fmt.Errorf("tenant uuid invalid")
		}

		tenantItem, err := loadTenantSnapshot(ctx, tenantRepo, tenantUUID)
		if err != nil {
			return err
		}
		if tenantItem.Status != tenantmodel.TenantStatusActive {
			return fmt.Errorf("tenant disabled")
		}

		userItem, err := loadUserSnapshot(ctx, userRepo, claims.UserID)
		if err != nil {
			return err
		}
		if userItem.Status != 1 {
			return fmt.Errorf("user disabled")
		}

		if claims.IsRoot {
			return nil
		}
		if claims.MemberID == 0 {
			return fmt.Errorf("member id missing")
		}
		memberItem, err := loadMemberSnapshot(ctx, memberRepo, claims.MemberID)
		if err != nil {
			return err
		}
		if memberItem.Status != 1 {
			return fmt.Errorf("member disabled")
		}
		if memberItem.UserID != claims.UserID {
			return fmt.Errorf("member user mismatch")
		}
		if !strings.EqualFold(strings.TrimSpace(memberItem.TenantUUID), tenantUUID) {
			return fmt.Errorf("member tenant mismatch")
		}
		return nil
	}
}

func validateSTSRouteOnly(ctx context.Context, claims *reqctx.CoreXClaims) error {
	if claims == nil {
		return fmt.Errorf("claims missing")
	}
	if !isPowerXAPISTSClaims(claims) {
		return nil
	}
	if isSTSAllowedRequestPath(ctx) {
		if _, err := reqctx.RequireTenantUUID(ctx); err != nil {
			return fmt.Errorf("tenant uuid missing")
		}
		return nil
	}
	return fmt.Errorf("sts token not allowed for this route")
}

func isPowerXAPISTSClaims(claims *reqctx.CoreXClaims) bool {
	if claims == nil || !strings.EqualFold(strings.TrimSpace(claims.Issuer), "powerx-sts") {
		return false
	}
	for _, aud := range claims.Audience {
		if strings.EqualFold(strings.TrimSpace(aud), "powerx:api") {
			return true
		}
	}
	return false
}

func isSTSAllowedRequestPath(ctx context.Context) bool {
	path := strings.TrimSpace(reqctx.GetRequestPath(ctx))
	path = strings.TrimSuffix(path, "/")
	return strings.HasSuffix(path, "/admin/runtime/ws-bus/grant") ||
		strings.HasSuffix(path, "/admin/runtime/ws-bus/publish") ||
		strings.Contains(path, "/admin/runtime/task-queue/") ||
		strings.HasSuffix(path, "/notifications/test") ||
		strings.HasSuffix(path, "/tenant/invocations") ||
		strings.HasSuffix(path, "/tenant/invocations/stream") ||
		isSTSCoreCapabilityPath(path)
}

func isSTSCoreCapabilityPath(path string) bool {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for start := range parts {
		if start > 0 && strings.EqualFold(parts[start-1], "admin") {
			continue
		}
		for _, pattern := range stsAllowedCoreCapabilityRoutes {
			if matchSTSRoutePattern(parts[start:], pattern) {
				return true
			}
		}
	}
	return false
}

func matchSTSRoutePattern(pathParts []string, pattern string) bool {
	patternParts := strings.Split(strings.Trim(pattern, "/"), "/")
	if len(pathParts) != len(patternParts) {
		return false
	}
	for i := range patternParts {
		actual := strings.TrimSpace(pathParts[i])
		expected := strings.TrimSpace(patternParts[i])
		if actual == "" || expected == "" {
			return false
		}
		if strings.HasPrefix(expected, "{") && strings.HasSuffix(expected, "}") {
			continue
		}
		if !strings.EqualFold(actual, expected) {
			return false
		}
	}
	return true
}

func loadTenantSnapshot(ctx context.Context, repo *tenantrepo.TenantRepository, tenantUUID string) (*tenantSnapshot, error) {
	cacheStore := cache.GetCache()
	cacheKey := authTenantKey(tenantUUID)
	if cacheStore != nil {
		if raw, err := cacheStore.Get(ctx, cacheKey); err == nil && len(raw) > 0 {
			var out tenantSnapshot
			if json.Unmarshal(raw, &out) == nil {
				return &out, nil
			}
		}
	}

	item, err := repo.GetByUUID(ctx, tenantUUID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("tenant not found")
		}
		return nil, fmt.Errorf("load tenant failed")
	}
	out := &tenantSnapshot{Status: item.Status}
	if cacheStore != nil {
		if payload, marshalErr := json.Marshal(out); marshalErr == nil {
			_ = cacheStore.Set(ctx, cacheKey, payload, authSubjectCacheTTL)
		}
	}
	return out, nil
}

func loadUserSnapshot(ctx context.Context, repo *iamrepo.UserRepository, userID uint64) (*userSnapshot, error) {
	cacheStore := cache.GetCache()
	cacheKey := middleware.KUser(userID)
	if cacheStore != nil {
		if raw, err := cacheStore.Get(ctx, cacheKey); err == nil && len(raw) > 0 {
			var out userSnapshot
			if json.Unmarshal(raw, &out) == nil {
				return &out, nil
			}
		}
	}

	item, err := repo.FindByID(ctx, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("load user failed")
	}
	out := &userSnapshot{Status: item.Status}
	if cacheStore != nil {
		if payload, marshalErr := json.Marshal(out); marshalErr == nil {
			_ = cacheStore.Set(ctx, cacheKey, payload, authSubjectCacheTTL)
		}
	}
	return out, nil
}

func loadMemberSnapshot(ctx context.Context, repo *iamrepo.MemberRepository, memberID uint64) (*memberSnapshot, error) {
	cacheStore := cache.GetCache()
	cacheKey := middleware.KMember(memberID)
	if cacheStore != nil {
		if raw, err := cacheStore.Get(ctx, cacheKey); err == nil && len(raw) > 0 {
			var out memberSnapshot
			if json.Unmarshal(raw, &out) == nil {
				return &out, nil
			}
		}
	}

	item, err := repo.FindByID(ctx, memberID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("member not found")
		}
		return nil, fmt.Errorf("load member failed")
	}
	out := &memberSnapshot{
		Status:     item.Status,
		UserID:     item.UserID,
		TenantUUID: strings.TrimSpace(item.TenantUUID),
	}
	if cacheStore != nil {
		if payload, marshalErr := json.Marshal(out); marshalErr == nil {
			_ = cacheStore.Set(ctx, cacheKey, payload, authSubjectCacheTTL)
		}
	}
	return out, nil
}

func authTenantKey(tenantUUID string) string {
	return "auth:tenant_uuid:" + strings.ToLower(strings.TrimSpace(tenantUUID))
}

func registerAuthSubjectCacheInvalidation(db *gorm.DB) {
	if db == nil {
		return
	}
	authCacheCallbackOnce.Do(func() {
		updateHook := db.Callback().Update().After("gorm:after_update")
		_ = updateHook.Register("powerx:auth_cache_invalidate:update", invalidateAuthCacheByStatement)
		deleteHook := db.Callback().Delete().After("gorm:after_delete")
		_ = deleteHook.Register("powerx:auth_cache_invalidate:delete", invalidateAuthCacheByStatement)
	})
}

func invalidateAuthCacheByStatement(tx *gorm.DB) {
	if tx == nil || tx.Statement == nil {
		return
	}
	store := cache.GetCache()
	if store == nil {
		return
	}
	table := strings.ToLower(strings.TrimSpace(tx.Statement.Table))
	if table == "" {
		return
	}
	if !strings.HasSuffix(table, "iam_user") &&
		!strings.HasSuffix(table, "iam_member") &&
		!strings.HasSuffix(table, "iam_tenant") {
		return
	}
	invalidateByReflectValue(tx.Statement.Context, store, table, tx.Statement.ReflectValue)
}

func invalidateByReflectValue(ctx context.Context, store cache.ICache, table string, value reflect.Value) {
	if !value.IsValid() {
		return
	}
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return
		}
		invalidateByReflectValue(ctx, store, table, value.Elem())
		return
	}
	if value.Kind() == reflect.Slice || value.Kind() == reflect.Array {
		for i := 0; i < value.Len(); i++ {
			invalidateByReflectValue(ctx, store, table, value.Index(i))
		}
		return
	}
	if value.Kind() != reflect.Struct {
		return
	}

	switch {
	case strings.HasSuffix(table, "iam_user"):
		if field := value.FieldByName("ID"); field.IsValid() {
			if userID, ok := toUint64(field); ok && userID > 0 {
				_ = store.Delete(ctx, middleware.KUser(userID))
			}
		}
	case strings.HasSuffix(table, "iam_member"):
		if field := value.FieldByName("ID"); field.IsValid() {
			if memberID, ok := toUint64(field); ok && memberID > 0 {
				_ = store.Delete(ctx, middleware.KMember(memberID))
			}
		}
	case strings.HasSuffix(table, "iam_tenant"):
		if field := value.FieldByName("UUID"); field.IsValid() {
			raw := strings.TrimSpace(fmt.Sprint(field.Interface()))
			if raw != "" && raw != "<nil>" {
				_ = store.Delete(ctx, authTenantKey(raw))
			}
		}
	}
}

func toUint64(v reflect.Value) (uint64, bool) {
	if !v.IsValid() {
		return 0, false
	}
	switch v.Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint(), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if v.Int() < 0 {
			return 0, false
		}
		return uint64(v.Int()), true
	default:
		return 0, false
	}
}
