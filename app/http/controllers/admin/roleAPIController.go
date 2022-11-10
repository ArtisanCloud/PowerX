package admin

import (
	"errors"
	modelsPowerLib "github.com/ArtisanCloud/PowerLibs/v2/authorization/rbac/models"
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	"github.com/ArtisanCloud/PowerX/app/http/controllers/api"
	"github.com/ArtisanCloud/PowerX/app/http/request/admin/role"
	"github.com/ArtisanCloud/PowerX/app/service"
	globalConfig "github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/database/global"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type RoleAPIController struct {
	*api.APIController
	ServiceRole *service.RoleService
}

func NewRoleAPIController(context *gin.Context) (ctl *RoleAPIController) {

	return &RoleAPIController{
		APIController: api.NewAPIController(context),
		ServiceRole:   service.NewRoleService(context),
	}
}

func APIInsertRole(context *gin.Context) {
	ctl := NewRoleAPIController(context)

	params, _ := context.Get("role")
	role := params.(*modelsPowerLib.Role)

	defer api.RecoverResponse(context, "api.admin.role.insert")

	var err error

	// insert role
	err = ctl.ServiceRole.UpsertRoles(global.G_DBConnection.Omit(clause.Associations), []*modelsPowerLib.Role{role}, nil)
	if err != nil {
		ctl.RS.SetCode(globalConfig.API_ERR_CODE_FAIL_TO_INSERT_ROLE, globalConfig.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}
	ctl.RS.Success(context, role)

}

func APIGetRoleList(context *gin.Context) {
	ctl := NewRoleAPIController(context)

	params, _ := context.Get("params")
	para := params.(*role.ParaRoleList)

	defer api.RecoverResponse(context, "api.admin.role.list")

	// 当前版本，只需要给予一级角色
	roleList, err := ctl.ServiceRole.GetTreeList(global.G_DBConnection, nil, false)
	if err != nil {
		ctl.RS.SetCode(globalConfig.API_ERR_CODE_FAIL_TO_GET_ROLE_LIST, globalConfig.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}
	arrayList := []*object.HashMap{}
	for _, role := range roleList {
		data, _ := object.StructToHashMap(role)
		if para.WithEmployees {
			employees, err := ctl.ServiceRole.GetEmployeesByRoleIDs(global.G_DBConnection, []string{role.UniqueID})
			if err != nil {
				ctl.RS.SetCode(globalConfig.API_ERR_CODE_FAIL_TO_GET_EMPLOYEE_LIST, globalConfig.API_RETURN_CODE_ERROR, "", err.Error())
				panic(ctl.RS)
				return
			}
			(*data)["employees"] = employees
		}
		arrayList = append(arrayList, data)
	}

	ctl.RS.Success(context, arrayList)
}

func APIGetRoleDetail(context *gin.Context) {
	ctl := NewRoleAPIController(context)

	roleIDInterface, _ := context.Get("roleID")
	roleID := roleIDInterface.(string)

	defer api.RecoverResponse(context, "api.admin.role.detail")

	role, err := ctl.ServiceRole.GetRoleByID(global.G_DBConnection, roleID)
	if err != nil {
		ctl.RS.SetCode(globalConfig.API_ERR_CODE_FAIL_TO_GET_ROLE_DETAIL, globalConfig.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	if role == nil {
		ctl.RS.SetCode(globalConfig.API_ERR_CODE_FAIL_TO_GET_ROLE_DETAIL, globalConfig.API_RETURN_CODE_ERROR, "", "")
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, role)
}

func APIUpdateRole(context *gin.Context) {
	ctl := NewRoleAPIController(context)

	roleInterface, _ := context.Get("role")
	role := roleInterface.(*modelsPowerLib.Role)

	defer api.RecoverResponse(context, "api.admin.role.upsert")

	var err error
	err = ctl.ServiceRole.UpsertRoles(global.G_DBConnection, []*modelsPowerLib.Role{role}, []string{"name", "updated_at"})

	if err != nil {
		ctl.RS.SetCode(globalConfig.API_ERR_CODE_FAIL_TO_UPDATE_ROLE, globalConfig.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, role)

}

func APIDeleteRoles(context *gin.Context) {
	ctl := NewRoleAPIController(context)

	roleIDsInterface, _ := context.Get("roleIDs")
	roleIDs := roleIDsInterface.([]string)

	defer api.RecoverResponse(context, "api.admin.role.delete")

	err := global.G_DBConnection.Transaction(func(tx *gorm.DB) error {
		// 重新替换当前将要删除角色的用户，将其绑定到系统的"普通员工"角色上
		employeeIDs, err := ctl.ServiceRole.GetEmployeeIDsByRoleIDs(tx, roleIDs)
		if err != nil {
			return err
		}

		// 将用户绑定到系统的"普通员工"角色上
		if len(employeeIDs) > 0 {
			// 获取默认绑定的普通员工角色
			employeeRoleID := (&modelsPowerLib.Role{}).GetEmployeeComposedUniqueID()
			sysEmployeeRole, err := ctl.ServiceRole.GetRoleByID(tx, employeeRoleID)
			if err != nil {
				return err
			}
			if sysEmployeeRole == nil {
				return errors.New("系统角色未找到，请确保PowerX安装系统数据完整")
			}
			// 重新批量绑定用户到普通员工角色
			err = ctl.ServiceRole.BindRoleToEmployeesByEmployeeIDs(tx, sysEmployeeRole, employeeIDs)
			if err != nil {
				return err
			}
		}

		// 删除角色
		err = ctl.ServiceRole.DeleteRolesByIDs(tx, roleIDs)
		if err != nil {
			return err
		}

		return err
	})

	if err != nil {
		ctl.RS.SetCode(globalConfig.API_ERR_CODE_FAIL_TO_DELETE_ROLE, globalConfig.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return

	}

	ctl.RS.Success(context, err)
}

func APIBindRoleToEmployees(context *gin.Context) {
	ctl := NewRoleAPIController(context)

	roleInterface, _ := context.Get("role")
	role := roleInterface.(*modelsPowerLib.Role)
	employeeIDsInterface, _ := context.Get("employeeIDs")
	employeeIDs := employeeIDsInterface.([]string)

	defer api.RecoverResponse(context, "api.admin.role.detail")

	err := ctl.ServiceRole.BindRoleToEmployeesByEmployeeIDs(global.G_DBConnection, role, employeeIDs)
	if err != nil {
		ctl.RS.SetCode(globalConfig.API_ERR_CODE_FAIL_TO_BIND_ROLE_TO_EMPLOYEE, globalConfig.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, err)
}
