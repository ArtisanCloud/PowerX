package service

import (
	"encoding/json"
	"errors"
	modelPowerLib "github.com/ArtisanCloud/PowerLibs/v2/authorization/rbac/models"
	"github.com/ArtisanCloud/PowerLibs/v2/cache"
	"github.com/ArtisanCloud/PowerLibs/v2/database"
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	globalBootstrap "github.com/ArtisanCloud/PowerX/boostrap/cache/global"
	"github.com/ArtisanCloud/PowerX/boostrap/rbac/global"
	logger "github.com/ArtisanCloud/PowerX/loggerManager"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"time"
)

type RBACService struct {
	*Service
	PermissionModule *modelPowerLib.PermissionModule
	Permission       *modelPowerLib.Permission
}

func NewRBACService(ctx *gin.Context) (r *RBACService) {
	r = &RBACService{
		Service:    NewService(ctx),
		Permission: modelPowerLib.NewPermission(nil),
	}
	return r
}

func (srv *RBACService) GetPermissionModuleGroupList(db *gorm.DB) (groupedPermissionModules []*modelPowerLib.PermissionModule, err error) {

	groupedPermissionModules = []*modelPowerLib.PermissionModule{}

	return srv.PermissionModule.GetGroupList(db, nil, nil)

}

func (srv *RBACService) InsertPermissionModules(db *gorm.DB, permissionModules []*modelPowerLib.PermissionModule) error {

	return database.InsertModelsOnUniqueID(db, &modelPowerLib.PermissionModule{}, modelPowerLib.PERMISSION_MODULE_UNIQUE_ID, permissionModules)

}

func (srv *RBACService) UpsertPermissionModules(db *gorm.DB, permissionModules []*modelPowerLib.PermissionModule, fieldsToUpdate []string) error {

	return database.UpsertModelsOnUniqueID(db, &modelPowerLib.PermissionModule{}, modelPowerLib.PERMISSION_MODULE_UNIQUE_ID, permissionModules, fieldsToUpdate)

}

func (srv *RBACService) GetPermissionModuleByID(db *gorm.DB, permissionModuleID string) (module *modelPowerLib.PermissionModule, err error) {
	module = &modelPowerLib.PermissionModule{}

	condition := &map[string]interface{}{
		"index_permission_module_id": permissionModuleID,
	}
	err = database.GetFirst(db, condition, module, nil)
	if err != nil && errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return module, err

}

func (srv *RBACService) DeletePermissionModulesByIDs(db *gorm.DB, permissionModuleIDs []string) error {
	db = db.
		//Debug().
		Where("index_permission_module_id in (?)", permissionModuleIDs).
		Delete(&modelPowerLib.PermissionModule{})

	return db.Error
}

func (srv *RBACService) DeletePermissionModuleByID(db *gorm.DB, permissionModuleID string) error {
	db = db.
		//Debug().
		Where("index_permission_module_id", permissionModuleID).
		Delete(&modelPowerLib.PermissionModule{})

	return db.Error
}

func (srv *RBACService) GetPermissionModulesByIDs(db *gorm.DB, permissionModuleIDs []string) (modules []*modelPowerLib.PermissionModule, err error) {
	modules = []*modelPowerLib.PermissionModule{}

	if len(permissionModuleIDs) > 0 {
		db = db.
			//Debug().
			Where("index_permission_module_id in (?)", permissionModuleIDs).
			Find(&modules)
		err = db.Error
	}

	return modules, err
}

// ---------------------------------------------------------------------------------------------------------------------

func (srv *RBACService) GetCachedPermissionByResource(db *gorm.DB, uri string, action string) (permission *modelPowerLib.Permission, err error) {

	permission = &modelPowerLib.Permission{}
	cacheKey := srv.GetPermissionCacheKey(uri, action)

	result, err := globalBootstrap.G_CacheConnection.Remember(cacheKey, cache.SYSTEM_CACHE_TIMEOUT_MONTH*time.Second, func() (interface{}, error) {
		list, err := srv.GetPermissionByResource(db, uri, action)
		logger.Logger.Info("cached GetPermissionByResource")
		return list, err
	})

	if err == cache.ErrCacheMiss {
		permission = result.(*modelPowerLib.Permission)

	} else if err == nil {
		strCacheObject, err := json.Marshal(result)
		err = json.Unmarshal([]byte(strCacheObject), permission)
		if err != nil {
			return nil, err
		}
	}
	return permission, err

}

func (srv *RBACService) GetPermissionList(db *gorm.DB, preload []string) (permissions []*modelPowerLib.Permission, err error) {

	permissions = []*modelPowerLib.Permission{}

	db = db.
		Order("object_alias desc").
		Order("object_value asc")

	err = database.GetAllList(db, nil, &permissions, preload)

	return permissions, err

}

func (srv *RBACService) InsertPermissions(db *gorm.DB, permissions []*modelPowerLib.Permission) error {

	return database.InsertModelsOnUniqueID(db, &modelPowerLib.Permission{}, modelPowerLib.PERMISSION_UNIQUE_ID, permissions)

}
func (srv *RBACService) UpsertPermissions(db *gorm.DB, permissions []*modelPowerLib.Permission, fieldsToUpdate []string) error {

	return database.UpsertModelsOnUniqueID(db, &modelPowerLib.Permission{}, modelPowerLib.PERMISSION_UNIQUE_ID, permissions, fieldsToUpdate)

}

func (srv *RBACService) DeletePermissionsByIDs(db *gorm.DB, permissionIDs []string) error {
	db = db.
		//Debug().
		Where("index_permission_id in (?)", permissionIDs).
		Delete(&modelPowerLib.Permission{})

	return db.Error
}

func (srv *RBACService) DeletePermissionByID(db *gorm.DB, permissionID string) error {
	db = db.
		//Debug().
		Where("index_permission_id", permissionID).
		Delete(&modelPowerLib.Permission{})

	return db.Error
}

func (srv *RBACService) GetPermissionsByIDs(db *gorm.DB, permissionIDs []string) (permissions []*modelPowerLib.Permission, err error) {
	permissions = []*modelPowerLib.Permission{}

	if len(permissionIDs) > 0 {
		db = db.
			//Debug().
			Where("index_permission_id in (?)", permissionIDs).
			Find(&permissions)
		err = db.Error
	}

	return permissions, err
}

func (srv *RBACService) GetPermissionByResource(db *gorm.DB, objectValue string, action string) (permission *modelPowerLib.Permission, err error) {
	permission = &modelPowerLib.Permission{}

	condition := &map[string]interface{}{
		"object_value": objectValue,
		"action":       action,
	}
	preload := []string{"PermissionModule"}
	err = database.GetFirst(db, condition, permission, preload)
	if err != nil && errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return permission, err
}

func (srv *RBACService) GetPermissionByID(db *gorm.DB, permissionID string) (permission *modelPowerLib.Permission, err error) {
	permission = &modelPowerLib.Permission{}

	condition := &map[string]interface{}{
		"index_permission_id": permissionID,
	}
	preload := []string{"PermissionModule"}
	err = database.GetFirst(db, condition, permission, preload)
	if err != nil && errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return permission, err
}

// ---------------------------------------------------------------------------------------------------------------------
func (srv *RBACService) GetPolicyListGroupedByRole(role *modelPowerLib.Role) (policies *object.HashMap, err error) {
	policies = &object.HashMap{}
	var arrayPolicies [][]string
	if role != nil {
		arrayPolicies = global.G_Enforcer.GetFilteredPolicy(0, role.GetRBACRuleName())
	} else {
		arrayPolicies = global.G_Enforcer.GetPolicy()
	}

	for _, policyItem := range arrayPolicies {

		keyRole := policyItem[0]
		keyObject := policyItem[1]

		// 第一层角色，初始化
		if (*policies)[keyRole] == nil {
			(*policies)[keyRole] = &object.HashMap{}
		}

		// 获取当前记录对应的对象
		layerRole := (*policies)[keyRole].(*object.HashMap)
		if (*layerRole)[keyObject] == nil {
			(*layerRole)[keyObject] = &object.HashMap{}
		}

		// 当前角色对象的权限控制
		(*layerRole)[keyObject] = &object.HashMap{
			"control": policyItem[2],
		}

	}

	return policies, err
}

func (srv *RBACService) GetPolicyList(role *modelPowerLib.Role) (policies []*modelPowerLib.RolePolicy, err error) {
	policies = []*modelPowerLib.RolePolicy{}
	var arrayPolicies [][]string
	if role != nil {
		arrayPolicies = global.G_Enforcer.GetFilteredPolicy(0, role.GetRBACRuleName())
	} else {
		arrayPolicies = global.G_Enforcer.GetPolicy()
	}

	for _, policyItem := range arrayPolicies {
		policy := &modelPowerLib.RolePolicy{
			RoleID:   policyItem[0],
			ObjectID: policyItem[1],
			Control:  policyItem[2],
		}
		policies = append(policies, policy)
	}

	return policies, err
}

func (srv *RBACService) GetCachedPolicyListGroupedByRole(role *modelPowerLib.Role) (policies map[string][]*modelPowerLib.RolePolicy, err error) {

	policies = map[string][]*modelPowerLib.RolePolicy{}
	cacheKey := srv.GetPolicyListCacheKey(role)

	result, err := globalBootstrap.G_CacheConnection.Remember(cacheKey, cache.SYSTEM_CACHE_TIMEOUT_MONTH*time.Second, func() (interface{}, error) {
		list, err := srv.GetPolicyListGroupedByRole(role)
		logger.Logger.Info("cached GetPolicyListGroupedByRole")
		return list, err
	})

	if err == cache.ErrCacheMiss {
		policies = result.(map[string][]*modelPowerLib.RolePolicy)

	} else if err == nil {
		strCacheObject, err := json.Marshal(result)
		err = json.Unmarshal([]byte(strCacheObject), &policies)
		if err != nil {
			return nil, err
		}
	}
	return policies, err

}

func (srv *RBACService) GetCachedPolicyList(role *modelPowerLib.Role) (policies []*modelPowerLib.RolePolicy, err error) {

	policies = []*modelPowerLib.RolePolicy{}
	cacheKey := srv.GetPolicyListCacheKey(role)

	result, err := globalBootstrap.G_CacheConnection.Remember(cacheKey, cache.SYSTEM_CACHE_TIMEOUT_MONTH*time.Second, func() (interface{}, error) {
		list, err := srv.GetPolicyList(role)
		logger.Logger.Info("cached GetPolicyList")
		return list, err
	})

	if err == cache.ErrCacheMiss {
		policies = result.([]*modelPowerLib.RolePolicy)

	} else if err == nil {
		strCacheObject, err := json.Marshal(result)
		err = json.Unmarshal([]byte(strCacheObject), &policies)
		if err != nil {
			return nil, err
		}
	}
	return policies, err

}

func (srv *RBACService) ClearCachedPolicyList(role *modelPowerLib.Role) (err error) {

	cacheKey := srv.GetPolicyListCacheKey(role)

	err = globalBootstrap.G_CacheConnection.Delete(cacheKey)

	return err

}

func (srv *RBACService) GetPermissionCacheKey(uri string, action string) (cacheKey string) {

	cacheKey = "rbac.permission." + uri + "-" + action
	return cacheKey
}

func (srv *RBACService) GetPolicyListCacheKey(role *modelPowerLib.Role) (cacheKey string) {
	roleID := "all"
	if role != nil {
		roleID = role.UniqueID
	}
	cacheKey = "policy.role." + roleID + ".list"
	return cacheKey
}

func (srv *RBACService) UpsertPolicies(policies []*modelPowerLib.RolePolicy, needRefresh bool) (err error) {

	for _, policy := range policies {

		existPolicies := global.G_Enforcer.GetFilteredPolicy(0, policy.RoleID, policy.ObjectID)

		if global.G_Enforcer.HasPolicy(policy.RoleID, policy.ObjectID, modelPowerLib.RBAC_CONTROL_ALL) ||
			global.G_Enforcer.HasPolicy(policy.RoleID, policy.ObjectID, modelPowerLib.RBAC_CONTROL_WRITE) ||
			global.G_Enforcer.HasPolicy(policy.RoleID, policy.ObjectID, modelPowerLib.RBAC_CONTROL_READ) ||
			global.G_Enforcer.HasPolicy(policy.RoleID, policy.ObjectID, modelPowerLib.RBAC_CONTROL_DELETE) ||
			global.G_Enforcer.HasPolicy(policy.RoleID, policy.ObjectID, modelPowerLib.RBAC_CONTROL_NONE) {
			for _, existPolicy := range existPolicies {
				if existPolicy[2] != policy.Control {
					_, err = global.G_Enforcer.UpdatePolicy(existPolicy, []string{policy.RoleID, policy.ObjectID, policy.Control})
					if err != nil {
						return err
					}
				}
			}
		} else {
			_, err = global.G_Enforcer.AddPolicy(policy.RoleID, policy.ObjectID, policy.Control)
			if err != nil {
				return err
			}
		}

	}

	if needRefresh {
		err = srv.ClearCachedPolicyList(nil)
	}

	return err
}
