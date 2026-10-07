package http

import (
	"net/http"

	"github.com/ArtisanCloud/PowerX/config"
	runtimeidentity "github.com/ArtisanCloud/PowerX/internal/service/runtime_identity"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

// HealthHandler 健康检查处理器
func HealthHandler(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	version := ""
	installStatus := "installed"
	configured := true
	deploymentEnv := ""
	if cfg := config.GetGlobalConfig(); cfg != nil {
		version = cfg.EffectiveSystemVersion()
		installStatus = cfg.Install.EffectiveStatus()
		configured = installStatus == "installed"
		deploymentEnv = cfg.Deployment.Env
	}
	data := gin.H{
		"status":         "ok",
		"service":        "CoreX",
		"version":        version,
		"install_status": installStatus,
		"configured":     configured,
		"runtime_mode":   "powerx",
		"core_version":   version,
		"deployment_env": deploymentEnv,
	}
	if _, requested := c.Request.URL.Query()["plugin_id"]; requested {
		dto.RespondErrorFrom(c, dto.Wrap(runtimeidentity.Error(http.StatusBadRequest, "HEALTH_PLUGIN_QUERY_UNSUPPORTED"), dto.RuntimeIdentityErrorMessage(c.GetHeader("Accept-Language"), "HEALTH_PLUGIN_QUERY_UNSUPPORTED")))
		return
	}
	dto.ResponseSuccess(c, data)
}
