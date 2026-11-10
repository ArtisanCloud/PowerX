package router

import (
	"context"
	"strings"

	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

// ResolvePluginServiceTarget 仅由 Core 进程内分发器使用挂载表解析地址。
// 与 Admin 反代共用启用、排空守卫；不接收调用方 URL 或认证头。
func (r *DynamicRouter) ResolvePluginServiceTarget(ctx context.Context, tenant, pluginID, providerVersion, requestPath string) (string, error) {
	if r.tenantGuard == nil || r.usageGuard == nil {
		return "", dto.NewErrorWithCode(503, "CAPABILITY_PLUGIN_RUNTIME_UNAVAILABLE", "CAPABILITY_PLUGIN_RUNTIME_UNAVAILABLE", nil)
	}
	if err := r.tenantGuard(ctx, tenant, pluginID); err != nil {
		return "", err
	}
	if err := r.usageGuard(ctx, pluginID); err != nil {
		return "", err
	}
	r.mu.RLock()
	up, ok := r.apis[pluginID]
	r.mu.RUnlock()
	if !ok || up.target == nil {
		return "", dto.NewErrorWithCode(503, "CAPABILITY_PLUGIN_RUNTIME_UNAVAILABLE", "CAPABILITY_PLUGIN_RUNTIME_UNAVAILABLE", nil)
	}
	if (up.basePath != "" && !strings.HasPrefix(requestPath, strings.TrimRight(up.basePath, "/")+"/")) || (up.target.Path != "" && up.target.Path != "/") {
		return "", dto.NewErrorWithCode(503, "CAPABILITY_SERVICE_BINDING_INVALID", "CAPABILITY_SERVICE_BINDING_INVALID", nil)
	}
	u := *up.target
	u.Path = requestPath
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	u.User = nil
	return u.String(), nil
}
