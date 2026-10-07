package plugin

import (
	"github.com/ArtisanCloud/PowerX/config"
	identity "github.com/ArtisanCloud/PowerX/internal/service/runtime_identity"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

// The route is protected by user JWT and pluginTenantAdminMiddleware.
func PluginRuntimeIdentityHandler(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var info identity.CoreInfo
	if cfg := config.GetGlobalConfig(); cfg != nil {
		info = identity.CoreInfo{Version: cfg.EffectiveSystemVersion(), DeploymentEnv: cfg.Deployment.Env, Ready: config.ValidateDeploymentEnv(cfg.Deployment.Env) == nil, Manager: tryGetPluginManager}
	}
	item, err := identity.NewService(info).Lookup(c.Request.Context(), c.Param("id"))
	if err != nil {
		dto.RespondErrorFrom(c, dto.Wrap(err, dto.RuntimeIdentityErrorMessage(c.GetHeader("Accept-Language"), dto.CodeOf(err))))
		return
	}
	dto.ResponseSuccess(c, gin.H{"item": item})
}
