package http

import (
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

// HealthHandler 健康检查处理器
func HealthHandler(c *gin.Context) {
	version := ""
	installStatus := "installed"
	configured := true
	if cfg := config.GetGlobalConfig(); cfg != nil {
		version = cfg.EffectiveSystemVersion()
		installStatus = cfg.Install.EffectiveStatus()
		configured = installStatus == "installed"
	}
	dto.ResponseSuccess(c, gin.H{
		"status":         "ok",
		"service":        "CoreX",
		"version":        version,
		"install_status": installStatus,
		"configured":     configured,
	})
}
