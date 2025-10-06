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
	// 关键：将 manifest 的 "/api/v1" 转成鉴权视角的 "/v1"
	policyBase := toPolicyHTTPBase(p.Endpoints.HTTPBasePath)
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

func normRes(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	return s
}
