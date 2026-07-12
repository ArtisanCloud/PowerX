package customer

import (
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/gin-gonic/gin"
)

func RegisterAPIRoutes(publicGroup, protectedGroup *gin.RouterGroup, deps *shared.Deps) {
	if protectedGroup == nil || deps == nil || deps.DB == nil {
		return
	}
	h := NewHandler(deps)
	g := protectedGroup.Group("/admin/customers")
	g.GET("/overview", h.Overview)
	g.GET("/accounts", h.ListAccounts)
	g.POST("/accounts", h.CreateAccount)
	g.PATCH("/accounts/:customer_uuid/status", h.UpdateStatus)
}
