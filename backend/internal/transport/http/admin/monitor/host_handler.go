package monitor

import (
	hostmonitor "github.com/ArtisanCloud/PowerX/internal/service/host_monitor"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

func HostOverview(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if !auth.IsPlatformRootSession(c.Request.Context()) {
		dto.ResponseError(c, 403, "host overview requires platform root session", nil)
		c.Abort()
		return
	}
	if len(c.Request.URL.Query()) != 0 {
		dto.ResponseError(c, 400, "host overview accepts no query parameters", nil)
		return
	}
	dto.ResponseSuccess(c, hostmonitor.NewService().Collect(c.Request.Context()))
}
