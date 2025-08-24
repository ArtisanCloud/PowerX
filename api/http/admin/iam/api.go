package iam

import (
	"github.com/ArtisanCloud/PowerX/internal/bootstrap"
	service "github.com/ArtisanCloud/PowerX/internal/service/iam"
	repoi "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	"github.com/gin-gonic/gin"
)

// 依赖注入（你项目已有 Deps 可替换）
func RegisterAPIRoutes(publicGroup *gin.RouterGroup, protectedGroup *gin.RouterGroup, deps *bootstrap.Deps) {
	orgSvc := service.NewOrgService(deps.DB)
	deptRepo := repoi.NewDepartmentRepository(deps.DB)

	hDept := NewDepartmentHandler(orgSvc, deptRepo)
	gDept := protectedGroup.Group("/admin/iam/departments")
	{
		gDept.GET("/tree", hDept.Tree)
		gDept.POST("", hDept.Create)
		gDept.PATCH("/:id", hDept.Update)
		gDept.DELETE("/:id", hDept.Delete)
	}

	hMember := NewMemberHandler(service.NewMemberService(deps.DB))
	gMember := protectedGroup.Group("/admin/iam/members")
	{
		gMember.GET("", hMember.List)
		gMember.GET("/:id", hMember.Get)
		gMember.POST("", hMember.Create)
		gMember.PATCH("/:id", hMember.Update)
		gMember.PUT("/:id/status", hMember.SetStatus)
		gMember.DELETE("/:id", hMember.Delete)
		gMember.PUT("/:id/restore", hMember.Restore)
		gMember.PUT("/:id/departments", hMember.PutDepartments)
		gMember.POST("/:id/force-logout", hMember.ForceMemberLogout)
	}
}
