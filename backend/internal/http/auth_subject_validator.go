package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	"gopkg.in/yaml.v3"
)

const authSubjectCacheTTL = 60 * time.Second

var (
	authCacheCallbackOnce sync.Once
	stsRouteCacheOnce     sync.Once
	stsRouteCache         []stsAllowedHTTPRoute
)

type stsRouteMatchMode string

const (
	stsRouteMatchSuffix      stsRouteMatchMode = "suffix"
	stsRouteMatchCorePattern stsRouteMatchMode = "core_pattern"
	stsRouteMatchExact       stsRouteMatchMode = "exact"
)

type stsAllowedHTTPRoute struct {
	Method  string
	Pattern string
	Match   stsRouteMatchMode
}

type stsCapabilityConfigFile struct {
	Capabilities []stsCapabilityConfigEntry `yaml:"capabilities"`
}

type stsCapabilityConfigEntry struct {
	Protocols []stsCapabilityProtocolEntry `yaml:"protocols"`
}

type stsCapabilityProtocolEntry struct {
	Channel      string `yaml:"channel"`
	Endpoint     string `yaml:"endpoint"`
	Method       string `yaml:"method"`
	ActorContext string `yaml:"actor_context"`
	STSDirect    bool   `yaml:"sts_direct"`
}

var stsStaticAllowedHTTPRoutes = []stsAllowedHTTPRoute{
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
	{Method: "POST", Pattern: "/admin/event-fabric/topics", Match: stsRouteMatchExact},
	{Method: "GET", Pattern: "/admin/scheduler/jobs", Match: stsRouteMatchExact},
	{Method: "POST", Pattern: "/admin/scheduler/jobs", Match: stsRouteMatchExact},
	{Method: "GET", Pattern: "/admin/scheduler/jobs/:job_id", Match: stsRouteMatchExact},
	{Method: "PATCH", Pattern: "/admin/scheduler/jobs/:job_id", Match: stsRouteMatchExact},
	{Method: "POST", Pattern: "/admin/scheduler/jobs/:job_id/trigger", Match: stsRouteMatchExact},
	{Method: "POST", Pattern: "/admin/scheduler/jobs/:job_id/pause", Match: stsRouteMatchExact},
	{Method: "POST", Pattern: "/admin/scheduler/jobs/:job_id/resume", Match: stsRouteMatchExact},
	{Method: "GET", Pattern: "/admin/scheduler/jobs/:job_id/runs", Match: stsRouteMatchExact},
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
	for _, route := range stsAllowedHTTPRoutes() {
		if route.matches(method, path) {
			return true
		}
	}
	return false
}

func stsAllowedHTTPRoutes() []stsAllowedHTTPRoute {
	stsRouteCacheOnce.Do(func() {
		routes := make([]stsAllowedHTTPRoute, 0, len(stsStaticAllowedHTTPRoutes)+64)
		seen := map[string]struct{}{}
		appendRoute := func(route stsAllowedHTTPRoute) {
			route.Method = strings.ToUpper(strings.TrimSpace(route.Method))
			route.Pattern = cleanSTSRoutePattern(route.Pattern)
			if route.Method == "" || route.Pattern == "" {
				return
			}
			key := route.Method + " " + route.Pattern + " " + string(route.Match)
			if _, ok := seen[key]; ok {
				return
			}
			seen[key] = struct{}{}
			routes = append(routes, route)
		}
		for _, route := range stsStaticAllowedHTTPRoutes {
			appendRoute(route)
		}
		for _, route := range loadSTSPlatformCapabilityRoutes() {
			appendRoute(route)
		}
		stsRouteCache = routes
	})
	return append([]stsAllowedHTTPRoute(nil), stsRouteCache...)
}

func loadSTSPlatformCapabilityRoutes() []stsAllowedHTTPRoute {
	dir, err := resolveSTSPlatformCapabilitiesDir()
	if err != nil {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	routes := make([]stsAllowedHTTPRoute, 0, 64)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.ToLower(entry.Name())
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		fileRoutes := loadSTSPlatformCapabilityRoutesFromFile(filepath.Join(dir, entry.Name()))
		routes = append(routes, fileRoutes...)
	}
	return routes
}

func loadSTSPlatformCapabilityRoutesFromFile(path string) []stsAllowedHTTPRoute {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var file stsCapabilityConfigFile
	if err := yaml.Unmarshal(raw, &file); err != nil {
		return nil
	}
	routes := make([]stsAllowedHTTPRoute, 0)
	for _, capability := range file.Capabilities {
		for _, protocol := range capability.Protocols {
			if !isSTSDirectProtocolBinding(protocol) {
				continue
			}
			method := strings.ToUpper(strings.TrimSpace(protocol.Method))
			pattern := normalizeSTSPlatformEndpoint(protocol.Endpoint)
			if method == "" || pattern == "" || isSTSPlatformEndpointDenied(method, pattern) {
				continue
			}
			routes = append(routes, stsAllowedHTTPRoute{
				Method:  method,
				Pattern: pattern,
				Match:   stsRouteMatchCorePattern,
			})
		}
	}
	return routes
}

func isSTSDirectProtocolBinding(protocol stsCapabilityProtocolEntry) bool {
	return strings.EqualFold(strings.TrimSpace(protocol.Channel), "rest") &&
		protocol.STSDirect &&
		strings.EqualFold(strings.TrimSpace(protocol.ActorContext), "service_actor")
}

func resolveSTSPlatformCapabilitiesDir() (string, error) {
	const defaultPlatformCapabilities = "backend/config/platform_capabilities"
	candidates := make([]string, 0, 5)
	if custom := strings.TrimSpace(os.Getenv("PLATFORM_CAPABILITIES_DIR")); custom != "" {
		candidates = append(candidates, custom)
	}
	candidates = append(candidates,
		filepath.Join(".", defaultPlatformCapabilities),
		filepath.Join("..", defaultPlatformCapabilities),
		filepath.Join("..", "..", "config", "platform_capabilities"),
		filepath.Join("..", "..", "..", defaultPlatformCapabilities),
	)
	if execPath, err := os.Executable(); err == nil {
		execDir := filepath.Dir(execPath)
		candidates = append(candidates,
			filepath.Join(execDir, defaultPlatformCapabilities),
			filepath.Join(execDir, "..", defaultPlatformCapabilities),
		)
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		abs, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		if info, err := os.Stat(abs); err == nil && info.IsDir() {
			return abs, nil
		}
	}
	return "", fmt.Errorf("platform capabilities directory not found")
}

func normalizeSTSPlatformEndpoint(endpoint string) string {
	endpoint = cleanSTSRoutePattern(endpoint)
	for _, prefix := range []string{"/api/v1", "/api"} {
		if endpoint == prefix {
			return "/"
		}
		if strings.HasPrefix(endpoint, prefix+"/") {
			return cleanSTSRoutePattern(strings.TrimPrefix(endpoint, prefix))
		}
	}
	return endpoint
}

func cleanSTSRoutePattern(pattern string) string {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return ""
	}
	if !strings.HasPrefix(pattern, "/") {
		pattern = "/" + pattern
	}
	for strings.Contains(pattern, "//") {
		pattern = strings.ReplaceAll(pattern, "//", "/")
	}
	if len(pattern) > 1 {
		pattern = strings.TrimRight(pattern, "/")
	}
	return pattern
}

func isSTSPlatformEndpointDenied(method string, pattern string) bool {
	_ = method
	pattern = strings.ToLower(cleanSTSRoutePattern(pattern))
	if pattern == "" {
		return true
	}
	if pattern == "/" || pattern == "/health" || pattern == "/healthz" {
		return true
	}
	firstSegment := strings.Trim(strings.Split(strings.Trim(pattern, "/"), "/")[0], " ")
	if strings.HasPrefix(firstSegment, ":") || (strings.HasPrefix(firstSegment, "{") && strings.HasSuffix(firstSegment, "}")) {
		return true
	}
	deniedPrefixes := []string{
		"/admin",
		"/internal",
		"/public",
		"/auth",
		"/setup",
	}
	for _, prefix := range deniedPrefixes {
		if pattern == prefix || strings.HasPrefix(pattern, prefix+"/") {
			return true
		}
	}
	deniedSegments := []string{
		"/debug",
		"/debug-",
		"/migration",
		"/root",
		"/drain",
		"/bootstrap",
		"/mock",
	}
	for _, segment := range deniedSegments {
		if strings.Contains(pattern, segment) {
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
	case stsRouteMatchExact:
		return isSTSExplicitCapabilityPath(path, route.Pattern)
	default:
		return false
	}
}

func isSTSExplicitCapabilityPath(path string, pattern string) bool {
	path = normalizeSTSPlatformEndpoint(path)
	pattern = cleanSTSRoutePattern(pattern)
	return matchSTSRoutePattern(strings.Split(strings.Trim(path, "/"), "/"), pattern)
}

func isSTSCoreCapabilityPath(path string, pattern string) bool {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for _, part := range parts {
		switch strings.ToLower(strings.TrimSpace(part)) {
		case "admin", "internal", "public", "auth", "setup":
			return false
		}
	}
	for start := range parts {
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
		if strings.HasPrefix(expected, ":") {
			continue
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
