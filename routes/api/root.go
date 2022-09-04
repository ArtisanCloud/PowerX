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

	apiInstallRouter := global.Router.Group("/root/api")
	{
		apiInstallRouter.Use(middleware.Installed, middleware.AuthRootAPI)
		{
			// 系统 - 启动安装
			apiInstallRouter.GET("/system/install", request.ValidateList, admin.APIGetCustomerList)
			apiInstallRouter.GET("/system/install/check", request.ValidateList, admin.APIGetCustomerList)

		}
	}

	apiRootRouter := global.Router.Group("/root/api")
	{
		apiRootRouter.Use(middleware.Installed, middleware.Maintenance, middleware.AuthRootAPI)
		{

			// root
			apiRootRouter.POST("/department/sync", wx.APISyncWXDepartments)
			apiRootRouter.POST("/employee/sync", admin.APISyncWXEmployees)
			apiRootRouter.POST("/customer/sync", admin.APISyncEmployeeAndWXAccount)

			// rbac
			apiRootRouter.GET("/permission/module/list", permissionModule.ValidatePermissionModuleList, admin.APIGetPermissionModuleList)
			apiRootRouter.GET("/permission/module/detail", permissionModule.ValidatePermissionModuleDetail, admin.APIGetPermissionModuleDetail)
			apiRootRouter.POST("/permission/module/create", permissionModule.ValidateInsertPermissionModule, admin.APIInsertPermissionModule)
			apiRootRouter.PUT("/permission/module/update", permissionModule.ValidateUpdatePermissionModule, admin.APIUpdatePermissionModule)
			apiRootRouter.DELETE("/permission/module/delete", permissionModule.ValidateDeletePermissionModule, admin.APIDeletePermissionModules)

			apiRootRouter.GET("/permission/list", permission.ValidatePermissionList, admin.APIGetPermissionList)
			apiRootRouter.GET("/permission/detail", permission.ValidatePermissionDetail, admin.APIGetPermissionDetail)
			apiRootRouter.POST("/permission/create", permission.ValidateInsertPermission, admin.APIInsertPermission)
			apiRootRouter.PUT("/permission/update", permission.ValidateUpdatePermission, admin.APIUpdatePermission)
			apiRootRouter.DELETE("/permission/delete", permission.ValidateDeletePermission, admin.APIDeletePermissions)
		}
	}

}
