package router

import (
	"context"
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

// --------- 1) 抽象：Authorizer（外部注入，实现从 DB/缓存等取“谁拥有什么”） ---------

type Authorizer interface {
	// 返回用户在某租户下对某插件拥有的权限（如 "note:read"）及策略版本（可为空）
	Permissions(ctx context.Context, tenantID, userID uint64, pluginID string) (perms []string, policyVersion string, err error)
	// 是否超管
	IsSuperAdmin(ctx context.Context, tenantID, userID uint64, roles []string) bool
}

// --------- 2) 路由 -> 所需权限 ---------

type Permission struct {
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

func (g *authzGate) CheckAndMint(ctx context.Context, pluginID, method, reqPath string, base reqctx.CoreXClaims) (string, bool, string) {
	// 超管直通
	if g.az != nil && g.az.IsSuperAdmin(ctx, base.TenantID, base.UserID, base.Roles) {
		tok, err := g.mintPluginToken(pluginID, base)
		if err != nil {
			return "", false, "mint token failed"
		}
		return tok, true, ""
	}

	// 找到路由所需权限（显式 or 自动）
	var need *Permission
	if pol := g.policies[pluginID]; pol != nil {
		need = pol.Required(method, reqPath)
		logger.DebugF(ctx, "[GATE-CHECK] plugin=%s method=%s reqPath=%s policy_base=%s routes=%d resources=%d need=%s",
			pluginID, method, reqPath, strings.TrimSpace(pol.HTTPBase), len(pol.Routes), len(pol.Resources), formatPermission(need))
	} else {
		logger.DebugF(ctx, "[GATE-CHECK] plugin=%s method=%s reqPath=%s policy=nil", pluginID, method, reqPath)
	}
	if need == nil {
		return "", false, "no permission rule for this route"
	}

	// 拉取授权（谁拥有什么）
	if g.az != nil {
		perms, _, err := g.az.Permissions(ctx, base.TenantID, base.UserID, pluginID)
		if err != nil {
			return "", false, "authz backend unavailable"
		}
		if !hasPermission(perms, *need) {
			logger.WarnF(ctx, "[GATE-CHECK] plugin=%s method=%s reqPath=%s deny=permission_miss need=%s user_perms=%s",
				pluginID, method, reqPath, formatPermission(need), strings.Join(normalizePermsForLog(perms), ","))
			return "", false, fmt.Sprintf("permission required: %s:%s", need.Resource, need.Action)
		}
	}

	// 通过 -> 给插件下发短期 Token
	tok, err := g.mintPluginToken(pluginID, base)
	if err != nil {
		return "", false, "mint token failed"
	}
	return tok, true, ""
}

func formatPermission(p *Permission) string {
	if p == nil {
		return "<nil>"
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
		UserID:     base.UserID,
		MemberID:   base.MemberID,
		IsRoot:     base.IsRoot,
	}

	aud := []string{"plugin:" + pluginID}
	secret := auth.GetJWTSecret()
	return auth.GenerateAccessJWT(c, g.issuer, aud, g.ttl, secret)
}

func hasPermission(userPerms []string, need Permission) bool {
	if len(userPerms) == 0 {
		return false
	}
	want := need.Resource + ":" + need.Action
	for _, p := range userPerms {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if p == want || p == "*" || p == need.Resource+":*" {
			return true
		}
		if strings.HasSuffix(p, ":*") && strings.TrimSuffix(p, ":*") == need.Resource {
			return true
		}
	}
	return false
}
