package integration_gateway

import (
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/gin-gonic/gin"
)

// RegisterTenantRoutes 注册租户侧集成网关 HTTP 路由。
func RegisterTenantRoutes(group *gin.RouterGroup, deps *shared.Deps) {
	if group == nil || deps == nil || deps.IntegrationGateway == nil || deps.IntegrationGateway.Tenant == nil {
		return
	}

	handler := &tenantHandler{svc: deps.IntegrationGateway.Tenant}
	routes := group.Group("/tenant/integration")
	routes.GET("/routes", handler.ListRoutes)
	routes.GET("/routes/:route_slug", handler.GetRoute)
	routes.POST("/routes/:route_slug/invoke", handler.InvokeRoute)
}
