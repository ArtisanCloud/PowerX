package authorization

import (
	"errors"
	modelPowerLib "github.com/ArtisanCloud/PowerLibs/v2/authorization/rbac/models"
	"github.com/ArtisanCloud/PowerLibs/v2/database"
	"github.com/ArtisanCloud/PowerLibs/v2/fmt"
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	"github.com/ArtisanCloud/PowerX/app/service"
	"github.com/ArtisanCloud/PowerX/boostrap"
	"github.com/ArtisanCloud/PowerX/boostrap/rbac"
	globalRBAC "github.com/ArtisanCloud/PowerX/boostrap/rbac/global"
	"github.com/ArtisanCloud/PowerX/config"
	database2 "github.com/ArtisanCloud/PowerX/database"
	globalDatabase "github.com/ArtisanCloud/PowerX/database/global"
	logger "github.com/ArtisanCloud/PowerX/loggerManager"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"path"
)

var rbacRoleDataPath = path.Join("configs", "rbac_role.json")
var rbacPermissionModuleDataPath = path.Join("configs", "rbac_permission_module.json")
var rbacPermissionDataPath = path.Join("configs", "rbac_permission.json")
var rbacPolicyRuleDataPath = path.Join("configs", "rbac_policy_rule.json")

func init() {
	var err error

	err = boostrap.InitConfig()
	if err != nil {
		panic(err)
	}

	// Initialize the logger
	err = logger.SetupLog(&config.G_AppConfigure.LogConfig)
	if err != nil {
		panic(err)
	}

	// 如果没有安装好系统，则无需在
	if !config.G_AppConfigure.SystemConfig.Installed {
		return
	}

	err = InitDatabase()
	if err != nil {
		panic(err)
	}

}

func InitDatabase() error {
	var err error
	err = config.LoadDatabaseConfig()
	if err != nil {
		return err
	}

	// Initialize the database
	err = database2.SetupDatabase(config.G_DBConfig)
	if err != nil {
		return err
	}

	// Initialize the RBAC Enforcer
	err = rbac.InitCasbin(globalDatabase.G_DBConnection)
	if err != nil {
		return err
	}

	return err
}

func RunAuthorization(cmd *cobra.Command, command string) {

	var err error
	err = InitDatabase()
	if err != nil {
		panic(err)
	}

	switch command {

	// permissions
	case "importRBACData":
		err = ImportRBACData(globalDatabase.G_DBConnection)

		break
	case "dumpRBACData":
		err = DumpRBACData(globalDatabase.G_DBConnection)

		break
	case "importPermissionModules":
		err = ImportPermissionModules(globalDatabase.G_DBConnection)

		break
	case "dumpPermissionModules":
		err = DumpPermissionModules(globalDatabase.G_DBConnection)

		break
	case "importPermissions":
		err = ImportPermissions(globalDatabase.G_DBConnection)

		break
	case "dumpPermissions":
		err = DumpPermissions(globalDatabase.G_DBConnection)

		break
	case "importPolicyRules":
		err = ImportPolicyRules(globalDatabase.G_DBConnection)

		break
	case "dumpPolicyRules":
		err = DumpPolicyRules(globalDatabase.G_DBConnection)

		break
	case "initRBACRolesAndPermissions":
		err = InitRBACRolesAndPermissions(globalDatabase.G_DBConnection)

		break
	case "initSystemRoles":
		err = InitSystemRoles(globalDatabase.G_DBConnection)

		break

	case "initPoliciesByRBACPermissions":
		err = InitPoliciesByRBACPermissions(globalDatabase.G_DBConnection)

		break

	// open apis
	case "convertRouts2OpenAPI":
		err = ConvertRouts2OpenAPI()

		break
	case "convertOpenAPI2Permissions":
		err = ConvertOpenAPI2Permissions()

		break
	case "convertRoutes2Permissions":
		err = ConvertRoutes2Permissions()

		break
	case "convertPermissions2OpenAPI":
		err = ConvertPermissions2OpenAPI()

		break
	default:
		printPrompt()
	}

	if err != nil {
		logger.Logger.Error("authorization command error:", zap.Any("err", err))
		return
	}

	fmt.Dump("run task done")
	return
}

func printPrompt() {
	println("please input available commands as below: ")

	println("convertRouts2OpenAPI")
	println("convertOpenAPI2Routs")
	println("convertRoutes2Permissions")
	println("convertPermissions2OpenAPI")

	println("importRBACData")
	println("dumpRBACData")
	println("importPermissionModules")
	println("dumpPermissionModules")

	println("initRBACRolesAndPermissions - initialize the RBAC data, it will generate table content of role, permission")
	println("initSystemRoles - initialize the system roles")
	println("initPoliciesByRBACPermissions - initialize the policies rules by RBAC permissions, call it after initRBAC and config permission modules by root")

}

// ---------------------------------------------------------------------------------------------------------------------
func ImportRBACData(db *gorm.DB) (err error) {

	err = ImportRoles(db)
	if err != nil {
		return err
	}

	err = ImportPermissionModules(db)
	if err != nil {
		return err
	}

	err = ImportPermissions(db)
	if err != nil {
		return err
	}

	err = ImportPolicyRules(db)
	if err != nil {
		return err
	}

	return err
}

func DumpRBACData(db *gorm.DB) (err error) {

	err = DumpRoles(db)
	if err != nil {
		return err
	}

	err = DumpPermissionModules(db)
	if err != nil {
		return err
	}

	err = DumpPermissions(db)
	if err != nil {
		return err
	}

	err = DumpPolicyRules(db)
	if err != nil {
		return err
	}

	return err
}

func ImportRoles(db *gorm.DB) (err error) {

	roles := []*modelPowerLib.Role{}
	err = object.LoadObjectFromFile(rbacRoleDataPath, &roles)
	if err != nil {
		return err
	}

	serviceRole := service.NewRoleService(nil)
	err = serviceRole.UpsertRoles(db, roles, nil)

	return err

}

func DumpRoles(db *gorm.DB) (err error) {

	roles := []*modelPowerLib.Role{}
	err = database.GetAllList(db, nil, &roles, nil)
	if err != nil {
		return err
	}
	if len(roles) <= 0 {
		return errors.New("权限未配置，请确保安装过程中已经配置权限")
	}

	err = object.SaveObjectToFile(roles, rbacRoleDataPath, 0644)

	return err

}

func ImportPermissionModules(db *gorm.DB) (err error) {

	permissionModules := []*modelPowerLib.PermissionModule{}
	err = object.LoadObjectFromFile(rbacPermissionModuleDataPath, &permissionModules)
	if err != nil {
		return err
	}

	serviceRBAC := service.NewRBACService(nil)
	err = serviceRBAC.UpsertPermissionModules(db, permissionModules, nil)

	return err

}

func DumpPermissionModules(db *gorm.DB) (err error) {

	permissionModules := []*modelPowerLib.PermissionModule{}
	err = database.GetAllList(db, nil, &permissionModules, nil)
	if err != nil {
		return err
	}
	if len(permissionModules) <= 0 {
		return errors.New("权限模块未配置，请确保安装过程中已经配置权限模块")
	}

	err = object.SaveObjectToFile(permissionModules, rbacPermissionModuleDataPath, 0644)

	return err

}

func ImportPermissions(db *gorm.DB) (err error) {

	permissions := []*modelPowerLib.Permission{}
	err = object.LoadObjectFromFile(rbacPermissionDataPath, &permissions)
	if err != nil {
		return err
	}

	serviceRBAC := service.NewRBACService(nil)
	err = serviceRBAC.UpsertPermissions(db, permissions, nil)

	return err

}

func DumpPermissions(db *gorm.DB) (err error) {

	permissions := []*modelPowerLib.Permission{}
	err = database.GetAllList(db, nil, &permissions, nil)
	if err != nil {
		return err
	}
	if len(permissions) <= 0 {
		return errors.New("权限未配置，请确保安装过程中已经配置权限")
	}

	err = object.SaveObjectToFile(permissions, rbacPermissionDataPath, 0644)

	return err

}

func ImportPolicyRules(db *gorm.DB) (err error) {

	policyList := []*modelPowerLib.RolePolicy{}
	err = object.LoadObjectFromFile(rbacPolicyRuleDataPath, &policyList)
	if err != nil {
		return err
	}

	serviceRBAC := service.NewRBACService(nil)
	err = serviceRBAC.UpsertPolicies(policyList, false)

	return err

}

func DumpPolicyRules(db *gorm.DB) (err error) {

	serviceRBAC := service.NewRBACService(nil)
	policyList, err := serviceRBAC.GetPolicyList(nil)
	if err != nil {
		return err
	}
	if len(policyList) <= 0 {
		return errors.New("角色权限未配置，请确保安装过程中已经配置权限")
	}

	err = object.SaveObjectToFile(policyList, rbacPolicyRuleDataPath, 0644)

	return err

}

// ---------------------------------------------------------------------------------------------------------------------
func InitRBACRolesAndPermissions(db *gorm.DB) (err error) {
	// 初始化Route列表转化成权限对象
	err = ConvertRoutes2Permissions()
	if err != nil {
		return err
	}

	// Root初始化系统角色
	err = InitSystemRoles(db)
	if err != nil {
		return err
	}

	return err
}

func InitSystemRoles(db *gorm.DB) (err error) {

	serviceRole := service.NewRoleService(nil)

	arrayRoles := []string{
		modelPowerLib.ROLE_SUPER_ADMIN_NAME,
		modelPowerLib.ROLE_ADMIN_NAME,
		modelPowerLib.ROLE_EMPLOYEE_NAME,
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		roles := []*modelPowerLib.Role{}
		for _, strRole := range arrayRoles {
			role := modelPowerLib.NewRole(object.NewCollection(&object.HashMap{
				"name":     strRole,
				"parentID": nil,
				"type":     modelPowerLib.ROLE_TYPE_SYSTEM,
			}))
			roles = append(roles, role)

			if err != nil {
				return err
			}
		}

		err = serviceRole.UpsertRoles(tx, roles, nil)

		return nil
	})

	return err

}

// 给到SUPER_ADMIN使用，初始化项目的系统匹配规则
// 安装项目的时候，可以直接从LoadPolicyesBySQL()来实现
func InitPoliciesByRBACPermissions(db *gorm.DB) (err error) {

	// 加载所有权限模块
	serviceRBAC := service.NewRBACService(nil)
	permissionModules, err := serviceRBAC.GetPermissionModuleGroupList(db)
	if err != nil {
		return err
	}
	if len(permissionModules) <= 0 {
		return errors.New("权限模块未配置，请确保安装过程中已经配置权限模块")
	}

	// 加载所有角色
	serviceRole := service.NewRoleService(nil)
	roles, err := serviceRole.GetTreeList(db, nil, false)
	if err != nil {
		return err
	}
	if len(roles) <= 0 {
		return errors.New("系统角色未配置，请确保安装过程中已经配置系统角色")
	}
	//fmt.Dump(roles)
	for _, role := range roles {

		err = initRolePoliciesByRBACPermissions(role, permissionModules, modelPowerLib.RBAC_CONTROL_ALL)
		if err != nil {
			logger.Logger.Error(err.Error())
			continue
		}
	}

	return err
}

func initRolePoliciesByRBACPermissions(role *modelPowerLib.Role, permissionModules []*modelPowerLib.PermissionModule, control string) (err error) {
	rules := [][]string{}

	var result bool

	// 三层模块结构，取第三层模块的权限
	// 业务模块层
	for _, permissionModule := range permissionModules {
		secondModules := permissionModule.Children
		// 功能模块层
		for _, secondModule := range secondModules {
			thirdModules := secondModule.Children
			for _, thirdModule := range thirdModules {
				// 功能模块
				fmt.Dump(role.GetRBACRuleName(), thirdModule.GetRBACRuleName())

				// 删除已有的权限
				existedRules := globalRBAC.G_Enforcer.GetFilteredPolicy(0, role.GetRBACRuleName(), thirdModule.GetRBACRuleName())
				for _, existedRule := range existedRules {
					_, err = globalRBAC.G_Enforcer.RemovePolicy(existedRule)
					if err != nil {
						logger.Logger.Error("删除已有规则" + err.Error())
					}
				}

				// 添加新的权限
				rule := []string{role.GetRBACRuleName(), thirdModule.GetRBACRuleName(), control}
				rules = append(rules, rule)
			}
		}
	}

	//fmt.Dump(rules)

	result, err = globalRBAC.G_Enforcer.AddPolicies(rules)
	if err != nil {
		err = errors.New("添加角色权限规则失败：" + err.Error())
		return err
	}
	if !result {
		err = errors.New("角色权限" + "role:" + role.Name + "添加结果失败，请确保数据为空")
		return err

	}

	return nil
}
