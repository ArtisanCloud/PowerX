package admin

import (
	modelsPowerLib "github.com/ArtisanCloud/PowerLibs/v2/authorization/rbac/models"
	"github.com/ArtisanCloud/PowerX/app/http/controllers/api"
	"github.com/ArtisanCloud/PowerX/app/service"
	globalConfig "github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/database/global"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm/clause"
)

type RBACAPIController struct {
	*api.APIController
	ServiceRBAC *service.RBACService
}

func NewRBACAPIController(context *gin.Context) (ctl *RBACAPIController) {

	return &RBACAPIController{
		APIController: api.NewAPIController(context),
		ServiceRBAC:   service.NewRBACService(context),
	}
}

func APIGetPermissionModuleList(context *gin.Context) {
	ctl := NewRBACAPIController(context)

	//params, _ := context.Get("params")
	//para := params.(*permission.ParaPermissionList)

	defer api.RecoverResponse(context, "api.admin.permission.module.list")

	arrayList, err := ctl.ServiceRBAC.GetPermissionModuleGroupList(global.G_DBConnection)
	if err != nil {
		ctl.RS.SetCode(globalConfig.API_ERR_CODE_FAIL_TO_GET_PERMISSION_MODULE_LIST, globalConfig.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, arrayList)
}

func APIGetPermissionModuleDetail(context *gin.Context) {
	ctl := NewRBACAPIController(context)

	permissionModuleIDInterface, _ := context.Get("permissionModuleID")
	permissionModuleID := permissionModuleIDInterface.(string)

	defer api.RecoverResponse(context, "api.admin.permission.module.detail")

	permission, err := ctl.ServiceRBAC.GetPermissionModuleByID(global.G_DBConnection, permissionModuleID)
	if err != nil {
		ctl.RS.SetCode(globalConfig.API_ERR_CODE_FAIL_TO_GET_PERMISSION_MODULE_DETAIL, globalConfig.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	if permission == nil {
		ctl.RS.SetCode(globalConfig.API_ERR_CODE_FAIL_TO_GET_PERMISSION_MODULE_DETAIL, globalConfig.API_RETURN_CODE_ERROR, "", "")
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, permission)
}

func APIInsertPermissionModule(context *gin.Context) {
	ctl := NewRBACAPIController(context)

	params, _ := context.Get("permissionModule")
	permissionModule := params.(*modelsPowerLib.PermissionModule)

	defer api.RecoverResponse(context, "api.admin.permission.module.insert")

	var err error

	// insert permission module
	err = ctl.ServiceRBAC.InsertPermissionModules(global.G_DBConnection.Omit(clause.Associations), []*modelsPowerLib.PermissionModule{permissionModule})
	if err != nil {
		ctl.RS.SetCode(globalConfig.API_ERR_CODE_FAIL_TO_INSERT_PERMISSION_MODULE, globalConfig.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}
	ctl.RS.Success(context, permissionModule)

}

func APIUpdatePermissionModule(context *gin.Context) {
	ctl := NewRBACAPIController(context)

	params, _ := context.Get("permissionModule")
	permissionModule := params.(*modelsPowerLib.PermissionModule)

	defer api.RecoverResponse(context, "api.admin.permission.module.insert")

	var err error

	// update permission module
	err = ctl.ServiceRBAC.UpsertPermissionModules(global.G_DBConnection.Omit(clause.Associations), []*modelsPowerLib.PermissionModule{permissionModule}, nil)
	if err != nil {
		ctl.RS.SetCode(globalConfig.API_ERR_CODE_FAIL_TO_UPDATE_PERMISSION_MODULE, globalConfig.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}
	ctl.RS.Success(context, permissionModule)

}

func APIDeletePermissionModules(context *gin.Context) {
	ctl := NewRBACAPIController(context)

	permissionModuleIDsInterface, _ := context.Get("permissionModuleIDs")
	permissionModuleIDs := permissionModuleIDsInterface.([]string)

	defer api.RecoverResponse(context, "api.admin.permission.module.delete")

	// 删除权限
	err := ctl.ServiceRBAC.DeletePermissionModulesByIDs(global.G_DBConnection, permissionModuleIDs)

	if err != nil {
		ctl.RS.SetCode(globalConfig.API_ERR_CODE_FAIL_TO_DELETE_PERMISSION_MODULE, globalConfig.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return

	}

	ctl.RS.Success(context, err)
}

// ------------------------------------------------------------

func APIGetPermissionList(context *gin.Context) {
	ctl := NewRBACAPIController(context)

	//params, _ := context.Get("params")
	//para := params.(*permission.ParaPermissionList)

	defer api.RecoverResponse(context, "api.admin.permission.list")

	arrayList, err := ctl.ServiceRBAC.GetPermissionList(global.G_DBConnection, nil)
	if err != nil {
		ctl.RS.SetCode(globalConfig.API_ERR_CODE_FAIL_TO_GET_PERMISSION_LIST, globalConfig.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, arrayList)
}

func APIGetPermissionDetail(context *gin.Context) {
	ctl := NewRBACAPIController(context)

	permissionIDInterface, _ := context.Get("permissionID")
	permissionID := permissionIDInterface.(string)

	defer api.RecoverResponse(context, "api.admin.permission.detail")

	permission, err := ctl.ServiceRBAC.GetPermissionByID(global.G_DBConnection, permissionID)
	if err != nil {
		ctl.RS.SetCode(globalConfig.API_ERR_CODE_FAIL_TO_GET_PERMISSION_DETAIL, globalConfig.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	if permission == nil {
		ctl.RS.SetCode(globalConfig.API_ERR_CODE_FAIL_TO_GET_PERMISSION_DETAIL, globalConfig.API_RETURN_CODE_ERROR, "", "")
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, permission)
}

func APIInsertPermission(context *gin.Context) {
	ctl := NewRBACAPIController(context)

	params, _ := context.Get("permission")
	permission := params.(*modelsPowerLib.Permission)

	defer api.RecoverResponse(context, "api.admin.permission.insert")

	var err error

	// insert permission
	err = ctl.ServiceRBAC.InsertPermissions(global.G_DBConnection.Omit(clause.Associations), []*modelsPowerLib.Permission{permission})
	if err != nil {
		ctl.RS.SetCode(globalConfig.API_ERR_CODE_FAIL_TO_INSERT_PERMISSION, globalConfig.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}
	ctl.RS.Success(context, permission)

}

func APIUpdatePermission(context *gin.Context) {
	ctl := NewRBACAPIController(context)

	permissionInterface, _ := context.Get("permission")
	permission := permissionInterface.(*modelsPowerLib.Permission)

	defer api.RecoverResponse(context, "api.admin.permission.upsert")

	var err error
	err = ctl.ServiceRBAC.UpsertPermissions(global.G_DBConnection, []*modelsPowerLib.Permission{permission}, []string{
		"updated_at",
		"object_alias",
		"description",
		"module_id",
	})

	if err != nil {
		ctl.RS.SetCode(globalConfig.API_ERR_CODE_FAIL_TO_UPDATE_PERMISSION, globalConfig.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, permission)

}

func APIDeletePermissions(context *gin.Context) {
	ctl := NewRBACAPIController(context)

	permissionIDsInterface, _ := context.Get("permissionIDs")
	permissionIDs := permissionIDsInterface.([]string)

	defer api.RecoverResponse(context, "api.admin.permission.delete")

	// 删除权限
	err := ctl.ServiceRBAC.DeletePermissionsByIDs(global.G_DBConnection, permissionIDs)

	if err != nil {
		ctl.RS.SetCode(globalConfig.API_ERR_CODE_FAIL_TO_DELETE_PERMISSION, globalConfig.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return

	}

	ctl.RS.Success(context, err)
}

// ------------------------------------------------------------

func APIGetPolicyGroupList(context *gin.Context) {
	ctl := NewRBACAPIController(context)

	defer api.RecoverResponse(context, "api.admin.policy.list")

	policies, err := ctl.ServiceRBAC.GetPolicyListGroupedByRole(nil)

	if err != nil {
		ctl.RS.SetCode(globalConfig.API_ERR_CODE_FAIL_TO_GET_ROLE_POLICY_LIST, globalConfig.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return

	}

	ctl.RS.Success(context, policies)
}

func APIGetPolicyList(context *gin.Context) {
	ctl := NewRBACAPIController(context)

	defer api.RecoverResponse(context, "api.admin.policy.list")

	policies, err := ctl.ServiceRBAC.GetPolicyList(nil)

	if err != nil {
		ctl.RS.SetCode(globalConfig.API_ERR_CODE_FAIL_TO_GET_ROLE_POLICY_LIST, globalConfig.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return

	}

	ctl.RS.Success(context, policies)
}

func APIUpdatePolicy(context *gin.Context) {
	ctl := NewRBACAPIController(context)

	policiesInterface, _ := context.Get("policies")
	policies := policiesInterface.([]*modelsPowerLib.RolePolicy)

	defer api.RecoverResponse(context, "api.admin.policy.update")

	err := ctl.ServiceRBAC.UpsertPolicies(policies, true)

	if err != nil {
		ctl.RS.SetCode(globalConfig.API_ERR_CODE_FAIL_TO_UPDATE_ROLE_POLICY, globalConfig.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, err)
}
