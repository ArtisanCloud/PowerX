package product

import (
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/internal/productinfo"
	marketsvc "github.com/ArtisanCloud/PowerX/internal/service/plugin_marketplace"
	"github.com/gin-gonic/gin"
)

// RegisterRoutes 是匿名只读声明入口，不授予 Admin 或服务态能力。
func RegisterRoutes(rg *gin.RouterGroup, deps *shared.Deps) {
	var svc *marketsvc.Service
	if deps != nil {
		svc = deps.PluginMarketplace
	}
	info := productinfo.Information{Product: "powerx", State: "missing"}
	if svc == nil {
		info = productinfo.LoadExecutable()
	}
	h := &Handler{svc: svc, fallback: info}
	rg.GET("/system/product", h.Product)
	rg.GET("/system/marketplace-proof", h.Proof)
}
