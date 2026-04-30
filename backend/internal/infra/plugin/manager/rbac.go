package manager

import (
	"strings"
	"time"

	pmrouter "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager/router"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
)

// 直接复用 router 包里的抽象，避免重复定义
type Authorizer = pmrouter.Authorizer
type Permission = pmrouter.Permission
type Policy = pmrouter.Policy

// 供 Manager -> Router 绑定授权提供者与短期 Token 配置
func BindAuthorizer(dr *pmrouter.DynamicRouter, az Authorizer, issuer string, ttl time.Duration) {
	if dr != nil {
		dr.BindAuthorizer(az, issuer, ttl)
	}
}

// PolicyFromPlugin：根据 manifest 的 HTTP 基础前缀 + 资源/动作清单
func PolicyFromPlugin(p plugin_mgr.Plugin) *pmrouter.Policy {
	policyBase := toPolicyHTTPBase(resolveRouteBasePath(p))
	pol := &pmrouter.Policy{
		Routes:      map[string]pmrouter.Permission{},
		HTTPBase:    policyBase,
		Resources:   map[string]map[string]bool{},
		DefaultOnly: false,
	}
	// ① health 检查：无条件允许（映射到一个虚拟 resource）
	pol.Routes["GET:/healthz"] = pmrouter.Permission{Resource: "system", Action: "read"}
	pol.Routes["HEAD:/healthz"] = pmrouter.Permission{Resource: "system", Action: "read"}

	// 兼容：若插件声明了 HTTP 基础前缀（如 /v1），也允许 "/<base>/healthz"
	if base := pol.HTTPBase; base != "" && base != "/" {
		// 简单拼接，确保只有一个斜杠
		withBase := base
		if !strings.HasSuffix(withBase, "/") {
			withBase += "/"
		}
		withBase += "healthz"
		pol.Routes["GET:"+withBase] = pmrouter.Permission{Resource: "system", Action: "read"}
		pol.Routes["HEAD:"+withBase] = pmrouter.Permission{Resource: "system", Action: "read"}
	}

	// ② 如果你也想给 /version 放行（可选）
	// pol.Routes["GET:/version"] = pmrouter.Permission{Resource: "system", Action: "read"}

	// ③ 把 manifest 声明的资源/动作注入
	for _, r := range p.RBAC.Resources {
		orig := strings.ToLower(strings.TrimSpace(r.Resource)) // e.g. "base:note"
		short := orig
		if i := strings.LastIndex(short, ":"); i >= 0 {
			short = short[i+1:] // → "note"
		}
		if short == "" {
			continue
		}

		// 可选：同时保留原键，防止别处依赖
		ensure := func(name string) {
			if _, ok := pol.Resources[name]; !ok {
				pol.Resources[name] = map[string]bool{}
			}
			for _, act := range r.Actions {
				pol.Resources[name][strings.ToLower(strings.TrimSpace(act))] = true
			}
		}
		ensure(short) // 用于自动推导能命中 "note"
		// ensure(orig) // 如果你希望兼容原来的 "base:note"，可以保留这一行
	}
	// ④ 编译插件显式路径权限（单一标准：basePath + permissions.path）
	for _, spec := range p.Permissions {
		relPath := strings.TrimSpace(spec.Path)
		resource := strings.ToLower(strings.TrimSpace(spec.Resource))
		if i := strings.LastIndex(resource, ":"); i >= 0 {
			resource = resource[i+1:]
		}
		if relPath == "" || resource == "" || len(spec.Actions) == 0 || policyBase == "" {
			continue
		}
		finalPath := joinPolicyPath(policyBase, relPath)
		for _, action := range spec.Actions {
			act := strings.ToLower(strings.TrimSpace(action))
			if act == "" {
				continue
			}
			for _, method := range actionToHTTPMethods(act) {
				pol.Routes[method+":"+finalPath] = pmrouter.Permission{Resource: resource, Action: act}
			}
		}
	}
	// ⑤ 宿主能力网关调试入口：为 POST /integration/capabilities/invoke 提供稳定映射
	// 兼容插件侧“功能页调试按钮”场景，避免因自动推导命中不到资源而 403。
	if base := pol.HTTPBase; base != "" {
		if res := pickCapabilityInvokeResource(pol.Resources); res != "" {
			invokePath := joinPolicyPath(base, "/integration/capabilities/invoke")
			pol.Routes["POST:"+invokePath] = pmrouter.Permission{Resource: res, Action: "create"}
		}
		// ⑥ 插件日志编排入口：为 /admin/runtime/logging/{policy,probe} 提供稳定映射
		// 避免由于路径首段是 "admin" 且插件未声明 admin 资源导致 no permission rule。
		if res := pickRuntimeLoggingResource(pol.Resources); res != "" {
			if readAct := pickRouteAction(pol.Resources[res], []string{"read", "view", "list", "query"}); readAct != "" {
				policyPath := joinPolicyPath(base, "/admin/runtime/logging/policy")
				pol.Routes["GET:"+policyPath] = pmrouter.Permission{Resource: res, Action: readAct}
			}
			if updateAct := pickRouteAction(pol.Resources[res], []string{"update", "edit", "write", "create", "read"}); updateAct != "" {
				policyPath := joinPolicyPath(base, "/admin/runtime/logging/policy")
				pol.Routes["PUT:"+policyPath] = pmrouter.Permission{Resource: res, Action: updateAct}
			}
			if probeAct := pickRouteAction(pol.Resources[res], []string{"create", "write", "update", "read"}); probeAct != "" {
				probePath := joinPolicyPath(base, "/admin/runtime/logging/probe")
				pol.Routes["POST:"+probePath] = pmrouter.Permission{Resource: res, Action: probeAct}
			}
		}
	}
	return pol
}

// 安装（或更新）某个插件的路由策略到动态路由器
func InstallPolicy(dr *pmrouter.DynamicRouter, pluginID string, pol *pmrouter.Policy) {
	if dr != nil && pol != nil {
		dr.InstallPolicy(pluginID, pol)
	}
}

// --------- helpers ---------
// 鉴权视角的 HTTPBase：把 manifest 的 "/api/v1" → "/v1"；"/api" → "/"
func toPolicyHTTPBase(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if s[0] != '/' {
		s = "/" + s
	}
	if s == "/api" {
		return "/"
	}
	if strings.HasPrefix(s, "/api/") {
		return "/" + strings.TrimPrefix(s, "/api/")
	}
	// 已经是 "/v1" 这类，原样返回
	return s
}

func resolveRouteBasePath(p plugin_mgr.Plugin) string {
	if p.Routes != nil && strings.TrimSpace(p.Routes.BasePath) != "" {
		return strings.TrimSpace(p.Routes.BasePath)
	}
	return strings.TrimSpace(p.Endpoints.HTTPBasePath)
}

func actionToHTTPMethods(action string) []string {
	switch action {
	case "read", "view", "list", "query":
		return []string{"GET", "HEAD"}
	case "create", "write":
		return []string{"POST"}
	case "update", "edit":
		return []string{"PUT", "PATCH"}
	case "delete", "remove":
		return []string{"DELETE"}
	default:
		return nil
	}
}

func normRes(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	return s
}

func pickCapabilityInvokeResource(resources map[string]map[string]bool) string {
	if len(resources) == 0 {
		return ""
	}
	prefer := []string{"integration", "capability", "template", "admin"}
	for _, name := range prefer {
		if _, ok := resources[name]; ok {
			return name
		}
	}
	for name := range resources {
		return name
	}
	return ""
}

func pickRuntimeLoggingResource(resources map[string]map[string]bool) string {
	if len(resources) == 0 {
		return ""
	}
	prefer := []string{"runtime", "logging", "logger", "system", "admin", "integration", "capability"}
	for _, name := range prefer {
		if _, ok := resources[name]; ok {
			return name
		}
	}
	for name := range resources {
		return name
	}
	return ""
}

func pickRouteAction(actions map[string]bool, prefer []string) string {
	if len(actions) == 0 {
		return ""
	}
	for _, act := range prefer {
		if actions[strings.ToLower(strings.TrimSpace(act))] {
			return strings.ToLower(strings.TrimSpace(act))
		}
	}
	for act := range actions {
		clean := strings.ToLower(strings.TrimSpace(act))
		if clean != "" {
			return clean
		}
	}
	return ""
}

func joinPolicyPath(base, suffix string) string {
	base = strings.TrimSpace(base)
	suffix = strings.TrimSpace(suffix)
	if base == "" {
		return suffix
	}
	if !strings.HasPrefix(base, "/") {
		base = "/" + base
	}
	if !strings.HasPrefix(suffix, "/") {
		suffix = "/" + suffix
	}
	if strings.HasSuffix(base, "/") {
		base = strings.TrimSuffix(base, "/")
	}
	return base + suffix
}
