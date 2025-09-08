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
// 构建一份“自动推导”的策略（无显式 Route 表时生效）
func PolicyFromPlugin(p plugin_mgr.Plugin) *pmrouter.Policy {
	pol := &pmrouter.Policy{
		Routes:      map[string]pmrouter.Permission{}, // 可选：你也可以在启动时往这里塞精确路由覆盖
		HTTPBase:    normBase(p.Endpoints.HTTPBasePath),
		Resources:   map[string]map[string]bool{},
		DefaultOnly: false,
	}

	// 注入资源 -> 动作集合
	for _, r := range p.RBAC.Resources {
		name := normRes(r.Resource)
		if name == "" {
			continue
		}
		if _, ok := pol.Resources[name]; !ok {
			pol.Resources[name] = map[string]bool{}
		}
		for _, act := range r.Actions {
			pol.Resources[name][strings.ToLower(strings.TrimSpace(act))] = true
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
func normBase(s string) string {
	if s == "" {
		return ""
	}
	if s[0] != '/' {
		return "/" + s
	}
	return s
}

func normRes(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	return s
}
