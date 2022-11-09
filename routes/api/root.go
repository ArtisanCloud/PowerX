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
		// 检查是否系统被安装过
		apiInstallRouter.GET("/system/install/check", rootAPI.APISystemCheckInstallation)

		// 安装系统接口
		apiInstallRouter.Use(middleware.CheckNotInstalled)
		{
			// 系统 - 启动安装
			apiInstallRouter.POST("/system/install", root.ValidateSystemInstall, rootAPI.APISystemInstall)
			apiInstallRouter.GET("/system/shutDown", rootAPI.APISystemShutDown)

		}

	}

	apiRootRouter := global.G_Router.Group("/root/api")
	{
		apiRootRouter.Use(middleware.CheckInstalled)
		{
			// 检查是否初始化过Root
			apiRootRouter.GET("/system/root/init/check", rootAPI.APIRootCheckInitialization)
			apiRootRouter.GET("/system/wx/config", rootAPI.APIWXConfig)
			// 初始化Root
			// --- 网页授权Root登陆，code换取访问token ---
			apiRootRouter.GET("/system/weCom/callback/authorized/root", root.ValidateInitRoot, rootAPI.APIInitRoot)

			apiRootRouter.GET("/ping", rootAPI.APIPing)
			apiRootRouter.GET("/detect/get", rootAPI.APIGetDetect)
			apiRootRouter.POST("/detect/post", rootAPI.APIPostDetect)
		}

		apiRootRouter.Use(middleware.CheckInstalled, middleware.Maintenance, middleware.AuthenticateRootByHeader)
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
