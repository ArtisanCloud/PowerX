package iam

import (
	"errors"
	"net/http"

	"github.com/ArtisanCloud/PowerX/internal/service"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h *RBACHandler) requireManagement(action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		actor, ok := h.actorContextFromGin(c)
		if !ok {
			c.Abort()
			return
		}
		if err := h.svc.AuthorizeManagement(c.Request.Context(), actor, action); err != nil {
			dto.RespondErrorFrom(c, err)
			c.Abort()
			return
		}
		c.Next()
	}
}

// Shared by production registration and signed-JWT authorization regression.
func registerRoleRoutes(protected *gin.RouterGroup, roles *RoleHandler, rbac *RBACHandler) {
	g := protected.Group("/admin/iam/roles")
	g.POST("", rbac.requireManagement("write"), roles.Create)
	g.GET("", rbac.requireManagement("read"), roles.List)
	g.GET("/:id", rbac.requireManagement("read"), roles.Get)
	g.PATCH("/:id", rbac.requireManagement("write"), roles.Update)
	g.DELETE("/:id", rbac.requireManagement("delete"), roles.Delete)
	g.POST("/:id/permissions/grant-ids", rbac.requireManagement("write"), rbac.GrantByIDs)
	g.POST("/:id/permissions/grant", rbac.requireManagement("write"), rbac.GrantByTriples)
	g.POST("/:id/permissions/revoke-ids", rbac.requireManagement("write"), rbac.RevokeByIDs)
	g.GET("/:id/permissions", rbac.requireManagement("read"), rbac.ListPermsOfRole)
	g.GET("/:id/permissions/ids", rbac.requireManagement("read"), rbac.ListPermIDs)
	g.PUT("/:id/permissions/set-ids", rbac.requireManagement("write"), rbac.SetPermIDs)
	g.POST("/:id/bind/member", rbac.requireManagement("bind"), rbac.BindRoleToMember)
	g.DELETE("/:id/bindings/:binding_id", rbac.requireManagement("bind"), rbac.UnbindRoleFromMember)
}

func respondRoleManagementError(c *gin.Context, message string, err error) {
	var app *dto.AppError
	if errors.As(err, &app) {
		dto.RespondErrorFrom(c, err)
		return
	}
	status := http.StatusBadRequest
	if errors.Is(err, service.ErrForbidden) {
		status = http.StatusForbidden
	} else if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, service.ErrRoleNotFound) {
		status = http.StatusNotFound
	}
	dto.ResponseError(c, status, message, err)
}
