package api

import (
	"github.com/ArtisanCloud/PowerX/app/http/controllers/admin"
	rootAPI "github.com/ArtisanCloud/PowerX/app/http/controllers/root"
	"github.com/ArtisanCloud/PowerX/app/http/controllers/wx"
	"github.com/ArtisanCloud/PowerX/app/http/middleware"
	"github.com/ArtisanCloud/PowerX/app/http/request/admin/permission"
	"github.com/ArtisanCloud/PowerX/app/http/request/admin/permission/permissionModule"
	root "github.com/ArtisanCloud/PowerX/app/http/request/root/install"
	"github.com/ArtisanCloud/PowerX/routes/global"
)

/* ------------------------------------------ root api ------------------------------------------*/

func InitRootAPIRoutes() {

	apiInstallRouter := global.G_Router.Group("/root/api")
	{
		apiInstallRouter.Use(middleware.CheckNotInstalled, middleware.AuthRootAPI)
		{
			// 系统 - 启动安装
			apiInstallRouter.POST("/system/install", root.ValidateSystemInstall, rootAPI.APISystemInstall)
			apiInstallRouter.GET("/system/shutDown", rootAPI.APISystemShutDown)
			apiInstallRouter.GET("/system/install/check", rootAPI.APISystemCheckInstallation)
		}
	}

	apiRootRouter := global.G_Router.Group("/root/api")
	{
		apiRootRouter.Use(middleware.CheckInstalled, middleware.Maintenance, middleware.AuthRootAPI)
		{
			apiRootRouter.GET("/ping", rootAPI.APIPing)
			apiRootRouter.GET("/detect/get", rootAPI.APIGetDetect)
			apiRootRouter.POST("/detect/post", rootAPI.APIPostDetect)

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
