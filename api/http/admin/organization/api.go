package organization

import (
	service "github.com/ArtisanCloud/PowerX/internal/service/organization"
	repoi "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func RegisterAPIRoutes(publicGroup *gin.RouterGroup, protectedGroup *gin.RouterGroup, db *gorm.DB) {
	// 依赖注入（你项目已有 Deps 可替换）
	orgSvc := service.NewOrgService(db)
	deptRepo := repoi.NewDepartmentRepository(db)

	h := NewDepartmentHandler(orgSvc, deptRepo)
	g := protectedGroup.Group("/admin/organization/departments")
	{
		g.GET("/tree", h.Tree)
		g.POST("", h.Create)
		g.PATCH("/:id", h.Update)
		g.DELETE("/:id", h.Delete)
	}
}
