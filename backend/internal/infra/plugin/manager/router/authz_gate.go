package router

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/auth"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

var (
	ErrPermissionSnapshotClaimsMissing = errors.New("GW_AUTHZ_PERMISSION_CLAIMS_MISSING")
	ErrPermissionSnapshotExpired       = errors.New("GW_AUTHZ_POLICY_VERSION_EXPIRED")
)

// --------- 1) 抽象：Authorizer（外部注入，实现从 DB/缓存等取“谁拥有什么”） ---------

type Authorizer interface {
	// 返回用户在某租户下对某插件拥有的权限（如 "note:read"）及策略版本（可为空）
	Permissions(ctx context.Context, tenantID, userID uint64, pluginID string) (perms []string, policyVersion string, err error)
	// 是否超管
	IsSuperAdmin(ctx context.Context, tenantID, userID uint64, roles []string) bool
}

type ClaimsAuthorizer interface {
	PermissionsForClaims(ctx context.Context, claims reqctx.CoreXClaims, pluginID string) (perms []string, policyVersion string, err error)
}

type RoutePermissionResolver interface {
	RoutePermission(ctx context.Context, pluginID, method, reqPath string) (*Permission, error)
}

// --------- 2) 路由 -> 所需权限 ---------

type Permission struct {
	Module   string `json:"module,omitempty" yaml:"module"`
	Resource string `json:"resource" yaml:"resource"`
	Action   string `json:"action"  yaml:"action"`
}

type Policy struct {
	// 显式路由规则（优先级最高）：key = "METHOD:/v1/notes/*"
	Routes map[string]Permission

	// 自动推导配置（无显式规则命中时使用）
	// HTTPBase: 插件自身声明的 API 前缀（manifest.endpoints.http_base_path）
	// Resources: 资源 -> 允许的动作集合（来自 manifest.rbac.resources[*].actions）
	HTTPBase  string
	Resources map[string]map[string]bool

	// DefaultOnly: 如果为 true，则忽略 Routes，仅使用自动推导（默认 false）
	DefaultOnly bool

	PublicRoutes []PublicRoute
}

type PublicRoute struct {
	Method string `json:"method" yaml:"method"`
	Path   string `json:"path" yaml:"path"`
}

func (p *Policy) Required(method, reqPath string) *Permission {
	// 1) 显式规则（精确/通配）
	if !p.DefaultOnly && len(p.Routes) > 0 {
		if perm, ok := p.Routes[method+":"+reqPath]; ok {
			return &perm
		}
		for k, perm := range p.Routes {
			m, pat := splitKey(k)
			if m != method && m != "*" {
				continue
			}
			if ok, _ := path.Match(pat, reqPath); ok {
				return &perm
			}
		}
	}

	// 2) 自动推导：method -> action；reqPath -> resource
	act := methodToAction(method)
	if act == "" {
		return nil
	}
	res := p.pickResourceFromPath(reqPath)
	if res == "" {
		return nil
	}
	if acts, ok := p.Resources[res]; ok {
		if acts[act] {
			return &Permission{Resource: res, Action: act}
		}
		// 动作同义词兜底（提高与插件 manifest 的契合度）
		for _, alt := range methodActionSynonyms(method) {
			if acts[alt] {
				return &Permission{Resource: res, Action: alt}
			}
		}
	}
	return nil
}

func splitKey(key string) (method, p string) {
	i := strings.IndexByte(key, ':')
	if i < 0 {
		return key, ""
	}
	return key[:i], key[i+1:]
}

func methodToAction(m string) string {
	switch strings.ToUpper(m) {
	case "GET", "HEAD":
		return "read"
	case "POST":
		return "create"
	case "PUT", "PATCH":
		return "update"
	case "DELETE":
		return "delete"
	default:
		return ""
	}
}

// methodActionSynonyms: 针对常见 HTTP 方法提供动作同义词回退
func methodActionSynonyms(m string) []string {
	switch strings.ToUpper(m) {
	case "GET", "HEAD":
		return []string{"view", "list"}
	case "POST":
		return []string{"write", "create"}
	case "PUT", "PATCH":
		return []string{"edit", "update"}
	case "DELETE":
		return []string{"remove", "delete"}
	default:
		return nil
	}
}

func (p *Policy) pickResourceFromPath(reqPath string) string {
	if reqPath == "" || reqPath == "/" {
		return ""
	}
	pathPart := reqPath

	// 去掉 http_base（如果存在）
	if p.HTTPBase != "" {
		base := p.HTTPBase
		if !strings.HasPrefix(base, "/") {
			base = "/" + base
		}
		if strings.HasPrefix(pathPart, base) {
			pathPart = strings.TrimPrefix(pathPart, base)
			if !strings.HasPrefix(pathPart, "/") {
				pathPart = "/" + pathPart
			}
		}
	}

	// 第一个段落即候选资源
	segs := strings.Split(strings.TrimLeft(pathPart, "/"), "/")
	if len(segs) == 0 || segs[0] == "" {
		return ""
	}
	raw := strings.ToLower(segs[0])

	// 简单单复数归一化：notes -> note, policies -> policy
	res := singularize(raw)

	// 优先匹配声明过的资源名；不匹配就尝试原词
	if _, ok := p.Resources[res]; ok {
		return res
	}
	if _, ok := p.Resources[raw]; ok {
		return raw
	}

	// 若该插件仅声明了唯一资源，则兜底用唯一资源
	if len(p.Resources) == 1 {
		for only := range p.Resources {
			return only
		}
	}
	return ""
}

func singularize(s string) string {
	switch {
	case strings.HasSuffix(s, "ies") && len(s) > 3:
		return s[:len(s)-3] + "y" // policies -> policy
	case strings.HasSuffix(s, "ses") && len(s) > 3:
		return s[:len(s)-2] // classes -> class（近似）
	case strings.HasSuffix(s, "s") && len(s) > 1:
		return s[:len(s)-1] // notes -> note
	default:
		return s
	}
}

// --------- 3) 预检 + 下发短期 Token（STS-lite） ---------

type authzGate struct {
	az       Authorizer
	issuer   string
	ttl      time.Duration
	policies map[string]*Policy // pluginID -> Policy
}

func newAuthzGate(az Authorizer, issuer string, ttl time.Duration) *authzGate {
	if ttl <= 0 {
		ttl = 60 * time.Second
	}
	return &authzGate{
		az:       az,
		issuer:   issuer,
		ttl:      ttl,
		policies: map[string]*Policy{},
	}
}

func (g *authzGate) InstallPolicy(pluginID string, pol *Policy) {
	if pol != nil {
		g.policies[pluginID] = pol
	}
}

func (g *authzGate) IsPublicRoute(pluginID, method, reqPath string) bool {
	if g == nil {
		return false
	}
	pol := g.policies[pluginID]
	if pol == nil {
		return false
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	reqPath = normalizePath(reqPath)
	for _, route := range pol.PublicRoutes {
		routeMethod := strings.ToUpper(strings.TrimSpace(route.Method))
		if routeMethod != "*" && routeMethod != method {
			continue
		}
		if normalizePath(route.Path) == reqPath {
			return true
		}
	}
	return false
}

func normalizePath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "/"
	}
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	return path.Clean(value)
}

func (g *authzGate) CheckAndMint(ctx context.Context, pluginID, method, reqPath string, base reqctx.CoreXClaims) (string, bool, string) {
	var need *Permission
	resolvedByAuthz := false
	if resolver, ok := g.az.(RoutePermissionResolver); ok {
		resolved, err := resolver.RoutePermission(ctx, pluginID, method, reqPath)
		if err != nil {
			return "", false, "authz route permission unavailable"
		}
		if resolved == nil {
			return "", false, "no registered permission binding for this route"
		}
		need = resolved
		resolvedByAuthz = true
		logger.DebugF(ctx, "[GATE-CHECK] plugin=%s method=%s reqPath=%s registered_permission=%s",
			pluginID, method, reqPath, formatPermission(need))
	}

	// 超管直通
	if base.IsRoot || (g.az != nil && g.az.IsSuperAdmin(ctx, base.TenantID, base.UserID, base.Roles)) {
		if need != nil {
			base.PermissionCodes = []string{formatPermission(need)}
			base.PermsHash = permissionCodesHash(base.PermissionCodes)
			base.PolicyVersion = permissionPolicyVersion(base.PermsHash)
		}
		tok, err := g.mintPluginToken(pluginID, base)
		if err != nil {
			return "", false, "mint token failed"
		}
		return tok, true, ""
	}

	// 找到路由所需权限（显式 or 自动）
	if resolvedByAuthz {
		// already resolved by registered plugin permission declarations
	} else if pol := g.policies[pluginID]; pol != nil {
		need = pol.Required(method, reqPath)
		logger.DebugF(ctx, "[GATE-CHECK] plugin=%s method=%s reqPath=%s policy_base=%s routes=%d resources=%d need=%s",
			pluginID, method, reqPath, strings.TrimSpace(pol.HTTPBase), len(pol.Routes), len(pol.Resources), formatPermission(need))
	} else {
		logger.DebugF(ctx, "[GATE-CHECK] plugin=%s method=%s reqPath=%s policy=nil", pluginID, method, reqPath)
	}
	if need == nil {
		return "", false, "no permission rule for this route"
	}
	if isRuntimeContractPermission(*need) {
		if !hasDelegatedRuntimeContext(base) {
			return "", false, "runtime contract tenant context missing"
		}
		base.PermissionCodes = []string{formatPermission(need)}
		base.PermsHash = permissionCodesHash(base.PermissionCodes)
		base.PolicyVersion = permissionPolicyVersion(base.PermsHash)
		tok, err := g.mintPluginToken(pluginID, base)
		if err != nil {
			return "", false, "mint token failed"
		}
		return tok, true, ""
	}

	// 拉取授权（谁拥有什么）
	if g.az != nil {
		var perms []string
		var policyVersion string
		var err error
		if claimsAuthorizer, ok := g.az.(ClaimsAuthorizer); ok {
			perms, policyVersion, err = claimsAuthorizer.PermissionsForClaims(ctx, base, pluginID)
		} else {
			perms, policyVersion, err = g.az.Permissions(ctx, base.TenantID, base.UserID, pluginID)
		}
		if err != nil {
			return "", false, "authz backend unavailable"
		}
		if !hasPermission(perms, *need) {
			logger.WarnF(ctx, "[GATE-CHECK] plugin=%s method=%s reqPath=%s deny=permission_miss need=%s user_perms=%s",
				pluginID, method, reqPath, formatPermission(need), strings.Join(normalizePermsForLog(perms), ","))
			return "", false, fmt.Sprintf("permission required: %s:%s", need.Resource, need.Action)
		}
		base.PermissionCodes = dedupeSortedPermissionCodes(perms)
		base.PermsHash = permissionCodesHash(base.PermissionCodes)
		if strings.TrimSpace(policyVersion) != "" {
			base.PolicyVersion = strings.TrimSpace(policyVersion)
		} else {
			base.PolicyVersion = permissionPolicyVersion(base.PermsHash)
		}
	}

	// 通过 -> 给插件下发短期 Token
	tok, err := g.mintPluginToken(pluginID, base)
	if err != nil {
		return "", false, "mint token failed"
	}
	return tok, true, ""
}

func isRuntimeContractPermission(need Permission) bool {
	module := strings.TrimSpace(need.Module)
	resource := strings.TrimSpace(need.Resource)
	action := strings.TrimSpace(need.Action)
	if action == "" {
		return false
	}
	if module == "runtime" && resource == "contract" {
		return true
	}
	return module == "" && resource == "contract" && action == "tenant_context"
}

func hasDelegatedRuntimeContext(base reqctx.CoreXClaims) bool {
	if strings.TrimSpace(base.TenantUUID) == "" {
		return false
	}
	hasUser := base.UserID > 0 || strings.TrimSpace(base.UserUUID) != ""
	hasMember := base.MemberID > 0 || strings.TrimSpace(base.MemberUUID) != ""
	return hasUser && hasMember
}

func formatPermission(p *Permission) string {
	if p == nil {
		return "<nil>"
	}
	if module := strings.TrimSpace(p.Module); module != "" {
		return module + "." + strings.TrimSpace(p.Resource) + ":" + strings.TrimSpace(p.Action)
	}
	return strings.TrimSpace(p.Resource) + ":" + strings.TrimSpace(p.Action)
}

func normalizePermsForLog(perms []string) []string {
	if len(perms) == 0 {
		return nil
	}
	out := make([]string, 0, len(perms))
	for _, p := range perms {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func (g *authzGate) mintPluginToken(pluginID string, base reqctx.CoreXClaims) (string, error) {
	if pluginID == "" {
		return "", errors.New("empty pluginID")
	}
	// 复制上游身份；如需把 perms/policyVersion 塞入 claims，先在 CoreXClaims 增字段即可
	c := reqctx.CoreXClaims{
		TenantUUID: base.TenantUUID,
		MemberUUID: base.MemberUUID,
		Platforms:  base.Platforms,
		TenantID:   base.TenantID,
		UserUUID:   base.UserUUID,
		UserID:     base.UserID,
		MemberID:   base.MemberID,
		Email:      strings.ToLower(strings.TrimSpace(base.Email)),
		Phone:      strings.TrimSpace(base.Phone),
		IsRoot:     base.IsRoot,
		Roles:      append([]string(nil), base.Roles...),

		PermissionCodes: append([]string(nil), base.PermissionCodes...),
		PolicyVersion:   strings.TrimSpace(base.PolicyVersion),
		PermsHash:       strings.TrimSpace(base.PermsHash),
	}

	aud := []string{"plugin:" + pluginID}
	secret := auth.GetJWTSecret()
	return auth.GenerateAccessJWT(c, g.issuer, aud, g.ttl, secret)
}

func dedupeSortedPermissionCodes(items []string) []string {
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

func permissionCodesHash(items []string) string {
	codes := dedupeSortedPermissionCodes(items)
	sum := sha256.Sum256([]byte(strings.Join(codes, "\n")))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func permissionPolicyVersion(permsHash string) string {
	permsHash = strings.TrimSpace(permsHash)
	if permsHash == "" {
		return ""
	}
	return "iam:" + permsHash
}

func ValidatePermissionSnapshot(claims reqctx.CoreXClaims, expectedPolicyVersion string) error {
	if len(claims.PermissionCodes) == 0 || strings.TrimSpace(claims.PermsHash) == "" || strings.TrimSpace(claims.PolicyVersion) == "" {
		return ErrPermissionSnapshotClaimsMissing
	}
	actualHash := permissionCodesHash(claims.PermissionCodes)
	if !strings.EqualFold(strings.TrimSpace(claims.PermsHash), actualHash) {
		return ErrPermissionSnapshotExpired
	}
	expectedPolicyVersion = strings.TrimSpace(expectedPolicyVersion)
	if expectedPolicyVersion != "" && strings.TrimSpace(claims.PolicyVersion) != expectedPolicyVersion {
		return ErrPermissionSnapshotExpired
	}
	if expectedPolicyVersion == "" && strings.TrimSpace(claims.PolicyVersion) != permissionPolicyVersion(actualHash) {
		return ErrPermissionSnapshotExpired
	}
	return nil
}

func hasPermission(userPerms []string, need Permission) bool {
	if len(userPerms) == 0 {
		return false
	}
	resource := strings.TrimSpace(need.Resource)
	action := strings.TrimSpace(need.Action)
	module := strings.TrimSpace(need.Module)
	want := resource + ":" + action
	if module != "" {
		want = module + "." + resource + ":" + action
	}
	for _, p := range userPerms {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if p == want || p == "*" || p == resource+":*" {
			return true
		}
		if module != "" && p == module+"."+resource+":*" {
			return true
		}
		if strings.HasSuffix(p, ":*") && strings.TrimSuffix(p, ":*") == resource {
			return true
		}
	}
	return false
}
