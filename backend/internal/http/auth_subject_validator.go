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

type stsRouteMatchMode string

const (
	stsRouteMatchSuffix      stsRouteMatchMode = "suffix"
	stsRouteMatchCorePattern stsRouteMatchMode = "core_pattern"
)

type stsAllowedHTTPRoute struct {
	Method  string
	Pattern string
	Match   stsRouteMatchMode
}

var stsAllowedHTTPRoutes = []stsAllowedHTTPRoute{
	{Method: "POST", Pattern: "/admin/runtime/ws-bus/grant", Match: stsRouteMatchSuffix},
	{Method: "POST", Pattern: "/admin/runtime/ws-bus/publish", Match: stsRouteMatchSuffix},
	{Method: "POST", Pattern: "/admin/runtime/task-queue/enqueue", Match: stsRouteMatchSuffix},
	{Method: "POST", Pattern: "/admin/runtime/task-queue/dequeue", Match: stsRouteMatchSuffix},
	{Method: "POST", Pattern: "/admin/runtime/task-queue/ack", Match: stsRouteMatchSuffix},
	{Method: "POST", Pattern: "/admin/runtime/task-queue/nack", Match: stsRouteMatchSuffix},
	{Method: "POST", Pattern: "/admin/runtime/task-queue/retry", Match: stsRouteMatchSuffix},
	{Method: "POST", Pattern: "/notifications/test", Match: stsRouteMatchSuffix},
	{Method: "POST", Pattern: "/tenant/invocations", Match: stsRouteMatchSuffix},
	{Method: "POST", Pattern: "/tenant/invocations/stream", Match: stsRouteMatchSuffix},
	{Method: "GET", Pattern: "/admin/tenants", Match: stsRouteMatchSuffix},
	{Method: "GET", Pattern: "/media/assets", Match: stsRouteMatchCorePattern},
	{Method: "POST", Pattern: "/media/assets", Match: stsRouteMatchCorePattern},
	{Method: "GET", Pattern: "/media/assets/{uuid}", Match: stsRouteMatchCorePattern},
	{Method: "PATCH", Pattern: "/media/assets/{uuid}", Match: stsRouteMatchCorePattern},
	{Method: "PUT", Pattern: "/media/assets/{uuid}", Match: stsRouteMatchCorePattern},
	{Method: "DELETE", Pattern: "/media/assets/{uuid}", Match: stsRouteMatchCorePattern},
	{Method: "POST", Pattern: "/media/assets/{uuid}/presign", Match: stsRouteMatchCorePattern},
	{Method: "POST", Pattern: "/media/assets/{uuid}/variants/{variant}", Match: stsRouteMatchCorePattern},
	{Method: "PUT", Pattern: "/media/assets/{uuid}/variants/{variant}", Match: stsRouteMatchCorePattern},
	{Method: "POST", Pattern: "/media/assets/{uuid}/variants/{variant}/presign", Match: stsRouteMatchCorePattern},
	{Method: "GET", Pattern: "/media/assets/{uuid}/variants/{variant}/resource", Match: stsRouteMatchCorePattern},
	{Method: "POST", Pattern: "/agents/invoke", Match: stsRouteMatchCorePattern},
	{Method: "GET", Pattern: "/agents/stream/sse", Match: stsRouteMatchCorePattern},
	{Method: "POST", Pattern: "/agents/sessions", Match: stsRouteMatchCorePattern},
	{Method: "POST", Pattern: "/ai/llm/invoke", Match: stsRouteMatchCorePattern},
	{Method: "POST", Pattern: "/ai/llm/stream", Match: stsRouteMatchCorePattern},
	{Method: "GET", Pattern: "/ai/llm/models", Match: stsRouteMatchCorePattern},
	{Method: "POST", Pattern: "/ai/llm/sessions", Match: stsRouteMatchCorePattern},
	{Method: "POST", Pattern: "/ai/llm/sessions/{session_id}/messages", Match: stsRouteMatchCorePattern},
	{Method: "POST", Pattern: "/ai/image/invoke", Match: stsRouteMatchCorePattern},
	{Method: "POST", Pattern: "/ai/video/invoke", Match: stsRouteMatchCorePattern},
	{Method: "POST", Pattern: "/ai/tts/invoke", Match: stsRouteMatchCorePattern},
	{Method: "POST", Pattern: "/ai/embedding/invoke", Match: stsRouteMatchCorePattern},
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
	method := strings.ToUpper(strings.TrimSpace(reqctx.GetRequestMethod(ctx)))
	if method == "" || path == "" {
		return false
	}
	for _, route := range stsAllowedHTTPRoutes {
		if route.matches(method, path) {
			return true
		}
	}
	return false
}

func (route stsAllowedHTTPRoute) matches(method string, path string) bool {
	if route.Method == "" || route.Pattern == "" || !strings.EqualFold(route.Method, method) {
		return false
	}
	switch route.Match {
	case stsRouteMatchSuffix:
		return strings.HasSuffix(path, route.Pattern)
	case stsRouteMatchCorePattern:
		return isSTSCoreCapabilityPath(path, route.Pattern)
	default:
		return false
	}
}

func isSTSCoreCapabilityPath(path string, pattern string) bool {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for start := range parts {
		if start > 0 && strings.EqualFold(parts[start-1], "admin") {
			continue
		}
		if matchSTSRoutePattern(parts[start:], pattern) {
			return true
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
