package http

import (
	"log"
	"net/http"
	"strings"

	"github.com/ArtisanCloud/PowerX/config"
	dto "github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

const (
	ErrCodeSystemNotInstalled = "SYSTEM_NOT_INSTALLED"
)

func InstallGuardMiddleware(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		if cfg == nil {
			c.Next()
			return
		}
		if cfg.Install.EffectiveStatus() == "installed" {
			c.Next()
			return
		}
		if cfg.Install.EffectiveLockMode() != "strict" {
			c.Next()
			return
		}
		if isInstallAllowedPath(cfg, c.Request.URL.Path) {
			c.Next()
			return
		}
		log.Printf("[install-guard] blocked path=%s status=%s lock_mode=%s", c.Request.URL.Path, cfg.Install.EffectiveStatus(), cfg.Install.EffectiveLockMode())

		err := dto.NewErrorWithCode(http.StatusServiceUnavailable, ErrCodeSystemNotInstalled, "系统尚未安装，请先完成 /setup 引导", nil)
		dto.ResponseError(c, http.StatusServiceUnavailable, "系统尚未安装", err)
		c.Abort()
	}
}

func isInstallAllowedPath(cfg *config.Config, path string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	if path == "/healthz" {
		return true
	}

	prefix := "/api/v1"
	if cfg != nil && strings.TrimSpace(cfg.Server.APIPrefix) != "" {
		prefix = strings.TrimSpace(cfg.Server.APIPrefix)
	}
	if !strings.HasPrefix(path, prefix) {
		// 非 API 前缀请求（例如静态资源）不做安装态硬拦截。
		return true
	}
	if path == prefix+"/health" {
		return true
	}
	if path == prefix+"/admin/setup/status" ||
		path == prefix+"/admin/setup/config" ||
		path == prefix+"/admin/setup/provision" ||
		path == prefix+"/admin/setup/complete" ||
		path == prefix+"/admin/setup/test/database" ||
		path == prefix+"/admin/setup/test/cache" ||
		path == prefix+"/admin/setup/llm/test-connection" ||
		path == prefix+"/admin/setup/llm/test-call" {
		return true
	}
	return false
}
