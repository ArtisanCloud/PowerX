package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"

	pmrouter "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager/router"
	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"gorm.io/gorm"
)

type pluginIAMAuthorizer struct {
	db *gorm.DB
}

func (a pluginIAMAuthorizer) Permissions(ctx context.Context, tenantID, userID uint64, pluginID string) ([]string, string, error) {
	return nil, "", fmt.Errorf("plugin authz requires claims context")
}

func (a pluginIAMAuthorizer) PermissionsForClaims(ctx context.Context, claims reqctx.CoreXClaims, pluginID string) ([]string, string, error) {
	if a.db == nil {
		return nil, "", fmt.Errorf("plugin authz db not configured")
	}
	tenantUUID := strings.TrimSpace(claims.TenantUUID)
	memberID := claims.MemberID
	if tenantUUID == "" || memberID == 0 {
		return nil, "", fmt.Errorf("plugin authz tenant_uuid and member_id required")
	}
	source := "plugin:" + strings.TrimSpace(pluginID)
	if source == "plugin:" {
		return nil, "", fmt.Errorf("plugin authz plugin_id required")
	}
	var rows []dbm.Permission
	err := a.db.WithContext(ctx).
		Table((&dbm.Permission{}).GetTableName(true)+" AS p").
		Select("p.*").
		Joins("JOIN "+(&dbm.RolePermission{}).GetTableName(true)+" rp ON rp.permission_id = p.id").
		Joins("JOIN ("+effectivePluginRoleIDsForMemberSQL()+") erb ON erb.role_id = rp.role_id", tenantUUID, dbm.SubMember, memberID, tenantUUID, memberID).
		Where("p.status = ? AND p.source = ?", dbm.PermissionStatusActive, source).
		Find(&rows).Error
	if err != nil {
		return nil, "", err
	}
	perms := make([]string, 0, len(rows))
	for _, row := range rows {
		code := permissionCodeFromIAMRow(row)
		if code == "" {
			continue
		}
		perms = append(perms, code)
		if row.Module == "" {
			perms = append(perms, strings.TrimSpace(row.Resource)+":"+strings.TrimSpace(row.Action))
		}
	}
	return dedupeSortedPluginPerms(perms), "", nil
}

func (a pluginIAMAuthorizer) IsSuperAdmin(_ context.Context, _, _ uint64, roles []string) bool {
	for _, r := range roles {
		rc := iam.RoleCode(strings.ToLower(strings.TrimSpace(r)))
		if rc == iam.CodeSystemAdmin {
			return true
		}
	}
	return false
}

func (a pluginIAMAuthorizer) RoutePermission(ctx context.Context, pluginID, method, reqPath string) (*pmrouter.Permission, error) {
	if a.db == nil {
		return nil, fmt.Errorf("plugin authz db not configured")
	}
	source := "plugin:" + strings.TrimSpace(pluginID)
	if source == "plugin:" {
		return nil, fmt.Errorf("plugin authz plugin_id required")
	}
	var rows []dbm.Permission
	if err := a.db.WithContext(ctx).
		Where("status = ? AND source = ?", dbm.PermissionStatusActive, source).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	method = normalizePluginRouteMethod(method)
	reqPath = normalizePluginRoutePath(reqPath)
	for _, row := range rows {
		var meta map[string]any
		_ = json.Unmarshal(row.Meta, &meta)
		permissionType := strings.TrimSpace(anyPluginAuthzString(meta["type"]))
		if permissionType != "api" && permissionType != "page" {
			continue
		}
		if !pluginPermissionMetaMatchesRoute(meta, method, reqPath) {
			continue
		}
		return &pmrouter.Permission{
			Module:   strings.TrimSpace(row.Module),
			Resource: strings.TrimSpace(row.Resource),
			Action:   strings.TrimSpace(row.Action),
		}, nil
	}
	return nil, nil
}

func effectivePluginRoleIDsForMemberSQL() string {
	tRB := (&dbm.RoleBinding{}).GetTableName(true)
	tMA := (&dbm.MemberAssignment{}).GetTableName(true)
	return `
		SELECT DISTINCT rb.role_id
		FROM ` + tRB + ` rb
		WHERE rb.tenant_uuid = ? AND rb.subject_type = ? AND rb.subject_id = ?
		UNION
		SELECT DISTINCT rb.role_id
		FROM ` + tRB + ` rb
		JOIN ` + tMA + ` ma
		  ON ma.tenant_uuid = rb.tenant_uuid
		 AND rb.subject_id = ma.dim_id
		 AND rb.subject_type = CASE ma.dim_type
		   WHEN 'ORG' THEN 'ORG_UNIT'
		   WHEN 'TEAM' THEN 'TEAM'
		   WHEN 'POSITION' THEN 'POSITION'
		   WHEN 'GROUP' THEN 'GROUP'
		 END
		WHERE rb.tenant_uuid = ? AND ma.member_id = ?`
}

func permissionCodeFromIAMRow(row dbm.Permission) string {
	module := strings.TrimSpace(row.Module)
	resource := strings.TrimSpace(row.Resource)
	action := strings.TrimSpace(row.Action)
	if resource == "" || action == "" {
		return ""
	}
	if module == "" {
		return resource + ":" + action
	}
	return module + "." + resource + ":" + action
}

func pluginPermissionMetaMatchesRoute(meta map[string]any, method, reqPath string) bool {
	bindings, ok := meta["protocol_bindings"].([]any)
	if !ok || len(bindings) == 0 {
		return false
	}
	for _, raw := range bindings {
		binding, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if strings.ToLower(strings.TrimSpace(anyPluginAuthzString(binding["channel"]))) != "rest" {
			continue
		}
		bindingMethod := normalizePluginRouteMethod(anyPluginAuthzString(binding["method"]))
		if bindingMethod != method {
			continue
		}
		bindingPath := normalizePluginRoutePath(anyPluginAuthzString(binding["path"]))
		if pluginRoutePathMatches(bindingPath, reqPath) {
			return true
		}
	}
	return false
}

func normalizePluginRouteMethod(method string) string {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "HEAD" {
		return "GET"
	}
	return method
}

func normalizePluginRoutePath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "/"
	}
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	return path.Clean(value)
}

func pluginRoutePathMatches(pattern, actual string) bool {
	pattern = normalizePluginRoutePath(pattern)
	actual = normalizePluginRoutePath(actual)
	if pattern == actual {
		return true
	}
	patternParts := strings.Split(strings.Trim(pattern, "/"), "/")
	actualParts := strings.Split(strings.Trim(actual, "/"), "/")
	if len(patternParts) != len(actualParts) {
		return false
	}
	for idx := range patternParts {
		pp := strings.TrimSpace(patternParts[idx])
		ap := strings.TrimSpace(actualParts[idx])
		if pp == "*" {
			continue
		}
		if strings.HasPrefix(pp, "{") && strings.HasSuffix(pp, "}") && ap != "" {
			continue
		}
		if pp != ap {
			return false
		}
	}
	return true
}

func anyPluginAuthzString(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func dedupeSortedPluginPerms(items []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, exists := seen[item]; exists {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}
