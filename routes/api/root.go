package api

import (
	"github.com/ArtisanCloud/PowerX/app/http/controllers/admin"
	"github.com/ArtisanCloud/PowerX/app/http/controllers/wx"
	"github.com/ArtisanCloud/PowerX/app/http/middleware"
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/ArtisanCloud/PowerX/app/http/request/admin/permission"
	"github.com/ArtisanCloud/PowerX/app/http/request/admin/permission/permissionModule"
	"github.com/ArtisanCloud/PowerX/routes/global"
)

/* ------------------------------------------ root api ------------------------------------------*/

func InitRootAPIRoutes() {

	apiRouter := global.Router.Group("/root/api")
	{
		apiRouter.Use(middleware.Maintenance, middleware.AuthRootAPI)
		{
			// 系统 - 启动安装
			apiRouter.GET("/system/install", request.ValidateList, admin.APIGetCustomerList)
			apiRouter.GET("/system/install/check", request.ValidateList, admin.APIGetCustomerList)

			// root
			apiRouter.POST("/department/sync", wx.APISyncWXDepartments)
			apiRouter.POST("/employee/sync", admin.APISyncWXEmployees)
			apiRouter.POST("/customer/sync", admin.APISyncEmployeeAndWXAccount)

			// rbac
			apiRouter.GET("/permission/module/list", permissionModule.ValidatePermissionModuleList, admin.APIGetPermissionModuleList)
			apiRouter.GET("/permission/module/detail", permissionModule.ValidatePermissionModuleDetail, admin.APIGetPermissionModuleDetail)
			apiRouter.POST("/permission/module/create", permissionModule.ValidateInsertPermissionModule, admin.APIInsertPermissionModule)
			apiRouter.PUT("/permission/module/update", permissionModule.ValidateUpdatePermissionModule, admin.APIUpdatePermissionModule)
			apiRouter.DELETE("/permission/module/delete", permissionModule.ValidateDeletePermissionModule, admin.APIDeletePermissionModules)

			apiRouter.GET("/permission/list", permission.ValidatePermissionList, admin.APIGetPermissionList)
			apiRouter.GET("/permission/detail", permission.ValidatePermissionDetail, admin.APIGetPermissionDetail)
			apiRouter.POST("/permission/create", permission.ValidateInsertPermission, admin.APIInsertPermission)
			apiRouter.PUT("/permission/update", permission.ValidateUpdatePermission, admin.APIUpdatePermission)
			apiRouter.DELETE("/permission/delete", permission.ValidateDeletePermission, admin.APIDeletePermissions)
		}
	}
}
