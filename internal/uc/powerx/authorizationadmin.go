package powerx

import (
	"PowerX/internal/config"
	"PowerX/internal/model/origanzation"
	"PowerX/internal/model/permission"
	"PowerX/internal/types/errorx"
	"PowerX/pkg/mapx"
	"PowerX/pkg/slicex"
	"context"
	"encoding/csv"
	sqladapter "github.com/Blank-Xu/sql-adapter"
	"github.com/casbin/casbin/v2"
	fileadapter "github.com/casbin/casbin/v2/persist/file-adapter"
	"github.com/pkg/errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"os"
	"path/filepath"
	"strings"
)

type AdminPermsUseCase struct {
	conf        *config.Config
	db          *gorm.DB
	Casbin      *casbin.Enforcer
	sqlAdapter  *sqladapter.Adapter
	fileAdapter *fileadapter.Adapter
	employee    *OrganizationUseCase
}

func NewAdminPermsUseCase(conf *config.Config, db *gorm.DB, employee *OrganizationUseCase) *AdminPermsUseCase {
	//casbin适配器
	sqlDB, _ := db.DB()
	a, err := sqladapter.NewAdapter(sqlDB, conf.PowerXDatabase.Driver, "casbin_policies")
	if err != nil {
		panic(err)
	}
	f := fileadapter.NewAdapter(filepath.Join(conf.EtcDir, "rbac_policy.csv"))
	e, err := casbin.NewEnforcer(filepath.Join(conf.EtcDir, "rbac_model.conf"), a)
	if err != nil {
		panic(err)
	}
	return &AdminPermsUseCase{
		conf:        conf,
		db:          db,
		Casbin:      e,
		sqlAdapter:  a,
		fileAdapter: f,
		employee:    employee,
	}
}

func (uc *AdminPermsUseCase) WithAuthMetadataCtxValue(ctx context.Context, md *permission.AdminAuthMetadata) context.Context {
	return context.WithValue(ctx, permission.AdminAuthMetadataKey{}, md)
}

func (uc *AdminPermsUseCase) AuthMetadataFromContext(ctx context.Context) (*permission.AdminAuthMetadata, error) {
	v, ok := ctx.Value(permission.AdminAuthMetadataKey{}).(*permission.AdminAuthMetadata)
	if !ok {
		return nil, errors.New("无法获取AuthMetadata")
	}
	return v, nil
}

func (uc *AdminPermsUseCase) Init() {
	var count int64

	// 初始化API
	if err := uc.db.Model(&permission.AdminAPI{}).Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "init api failed"))
	}
	if count == 0 {
		// api group
		initAPIGroup := func() {
			file, err := os.Open(filepath.Join(uc.conf.EtcDir, "api_group.csv"))
			if err != nil {
				panic(err)
			}
			csvReader := csv.NewReader(file)
			records, err := csvReader.ReadAll()
			if err != nil {
				panic(err)
			}

			var groups []permission.AdminAPIGroup
			for _, record := range records {
				groups = append(groups, permission.AdminAPIGroup{
					GroupCode: record[0],
					Prefix:    record[1],
					Name:      record[2],
					Desc:      record[3],
				})
			}

			for _, group := range groups {
				uc.db.Model(&permission.AdminAPIGroup{}).Where("group_code = ?", group.GroupCode).FirstOrCreate(&group)
			}
		}

		// api
		initAPI := func() {
			file, err := os.Open(filepath.Join(uc.conf.EtcDir, "api.csv"))
			if err != nil {
				panic(err)
			}
			csvReader := csv.NewReader(file)
			records, err := csvReader.ReadAll()
			if err != nil {
				panic(err)
			}

			var groups []*permission.AdminAPIGroup
			if err := uc.db.Model(&permission.AdminAPIGroup{}).Find(&groups).Error; err != nil {
				panic(errors.Wrap(err, "init api failed"))
			}

			groupMap := mapx.MapByFunc(groups, func(item *permission.AdminAPIGroup) (string, int64) {
				return item.GroupCode, item.Id
			})

			var apis []permission.AdminAPI
			for _, record := range records {
				var groupId int64
				if id, ok := groupMap[record[0]]; ok {
					groupId = id
				}
				apis = append(apis, permission.AdminAPI{
					API:     record[1],
					Method:  strings.ToUpper(record[2]),
					Name:    record[3],
					GroupId: groupId,
				})
			}

			for _, api := range apis {
				uc.db.Model(&permission.AdminAPI{}).Where(permission.AdminAPI{API: api.API, Method: api.Method}).FirstOrCreate(&api)
			}
		}

		initAPIGroup()
		initAPI()
	}

	// 初始化用户
	if err := uc.db.Model(&origanzation.Employee{}).Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "init role failed"))
	}
	if count == 0 {

		rooAccount := uc.conf.Root.Account
		if rooAccount == "" {
			rooAccount = "root"
		}
		rooPass := uc.conf.Root.Password
		if rooPass == "" {
			rooPass = "root"
		}
		rooName := uc.conf.Root.Name
		if rooName == "" {
			rooName = "超级管理员"
		}
		root := origanzation.Employee{
			Account:    rooAccount,
			Password:   rooPass,
			Name:       rooName,
			Status:     origanzation.EmployeeStatusEnabled,
			IsReserved: true,
		}
		root.HashPassword()
		if err := uc.db.Model(&origanzation.Employee{}).Create(&root).Error; err != nil {
			panic(errors.Wrap(err, "init root failed"))
		}
	}

	//// 初始化casbin策略
	//if err := uc.db.Model(&permission.EmployeeCasbinPolicy{}).Count(&count).Error; err != nil {
	//	panic(errors.Wrap(err, "init casbin policy failed"))
	//}
	//if count == 0 {
	//	uc.Casbin.SetAdapter(uc.fileAdapter)
	//	uc.Casbin.LoadPolicy()
	//	uc.Casbin.SetAdapter(uc.sqlAdapter)
	//	uc.Casbin.SavePolicy()
	//}

	// 初始化角色
	if err := uc.db.Model(&permission.AdminRole{}).Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "init role failed"))
	}
	if count == 0 {
		var roles []*permission.AdminRole

		var apis []*permission.AdminAPI
		if err := uc.db.Model(&permission.AdminAPI{}).Find(&apis).Error; err != nil {
			panic(errors.Wrap(err, "init role failed"))
		}

		roles = append(roles, &permission.AdminRole{
			RoleCode:   "admin",
			Name:       "管理员",
			Desc:       "管理员",
			AdminAPI:   apis,
			IsReserved: true,
		}, &permission.AdminRole{
			RoleCode:   "common_employee",
			Name:       "普通员工",
			Desc:       "普通员工",
			IsReserved: true,
		})
		for _, role := range roles {
			if err := uc.CreateRole(context.Background(), role); err != nil {
				panic(errors.Wrap(err, "init role failed"))
			}
		}
	}
}

func (uc *AdminPermsUseCase) FindOneRoleByRoleCode(ctx context.Context, roleCode string) (role *permission.AdminRole, err error) {
	err = uc.db.WithContext(ctx).Where(permission.AdminRole{RoleCode: roleCode}).
		Preload("AdminAPI").
		Preload("MenuNames").
		First(&role).Error
	if role.AdminAPI == nil {
		role.AdminAPI = make([]*permission.AdminAPI, 0)
	}
	if role.MenuNames == nil {
		role.MenuNames = make([]*permission.AdminRoleMenuName, 0)
	}
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未查找到角色")
		}
		panic(err)
	}
	return
}

func (uc *AdminPermsUseCase) FindAllRoles(ctx context.Context) (roles []*permission.AdminRole) {
	if err := uc.db.WithContext(ctx).Model(permission.AdminRole{}).Preload("MenuNames").Find(&roles).Error; err != nil {
		panic(err)
	}
	return
}

// CreateRole 创建角色
func (uc *AdminPermsUseCase) CreateRole(ctx context.Context, role *permission.AdminRole) error {
	var count int64
	if uc.db.Model(&permission.AdminRole{}).Where(permission.AdminRole{RoleCode: role.RoleCode}).Count(&count); count > 0 {
		return errorx.WithCause(errorx.ErrBadRequest, "角色已存在")
	}

	err := uc.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		tx.Model(&permission.AdminRole{}).Create(role)

		if len(role.AdminAPI) == 0 {
			return nil
		}

		apiIds := slicex.SlicePluck(role.AdminAPI, func(item *permission.AdminAPI) int64 {
			return item.Id
		})

		var apis []*permission.AdminAPI
		if err := tx.Model(&permission.AdminAPI{}).Where(apiIds).Find(&apis).Error; err != nil {
			return err
		}

		var policies [][]string
		for _, api := range apis {
			policies = append(policies, []string{role.RoleCode, api.API, api.Method})
		}

		if _, err := uc.Casbin.AddPolicies(policies); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		panic(err)
	}

	return nil
}

// PatchRoleByRoleId 通过角色ID更新角色
func (uc *AdminPermsUseCase) PatchRoleByRoleId(ctx context.Context, role *permission.AdminRole, roleId int64) {
	role.Id = roleId

	err := uc.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 获取 db 的 role MenuNames]
		var dbRole permission.AdminRole
		if err := tx.Model(&permission.AdminRole{}).Where(roleId).Preload("MenuNames").First(&dbRole).Error; err != nil {
			return err
		}

		// 根据 db 的 role MenuNames 设置 role MenuNames 已经存在的id，避免重复插入
		menuNameMap := make(map[string]int64)
		for _, menuName := range dbRole.MenuNames {
			menuNameMap[menuName.MenuName] = menuName.Id
		}
		for i := range role.MenuNames {
			if id, ok := menuNameMap[role.MenuNames[i].MenuName]; ok {
				role.MenuNames[i].Id = id
			}
		}

		// 删除旧的权限策略
		if _, err := uc.Casbin.RemoveFilteredNamedPolicy("p", 0, role.RoleCode); err != nil {
			return err
		}

		apiIds := slicex.SlicePluck(role.AdminAPI, func(item *permission.AdminAPI) int64 {
			return item.Id
		})

		var apis []*permission.AdminAPI
		if err := tx.Model(&permission.AdminAPI{}).Where(apiIds).Find(&apis).Error; err != nil {
			return err
		}

		// 生成新的权限策略
		var policies [][]string
		for _, api := range apis {
			policies = append(policies, []string{role.RoleCode, api.API, api.Method})
		}

		// 添加新的权限策略
		if _, err := uc.Casbin.AddPolicies(policies); err != nil {
			return err
		}

		// 更新角色信息, 忽略 AdminAPI 字段 ( 稍后replace )
		if err := tx.Omit("AdminAPI").Clauses(clause.Returning{}).Updates(&role).Error; err != nil {
			return err
		}

		// 替换角色的 api 权限展示数据
		err := tx.Model(&dbRole).Association("AdminAPI").Replace(role.AdminAPI)
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		panic(err)
	}
}

func (uc *AdminPermsUseCase) PatchRoleByRoleCode(ctx context.Context, role *permission.AdminRole, roleCode string) {
	var dbRole permission.AdminRole
	if err := uc.db.Where(&permission.AdminRole{RoleCode: roleCode}).Find(&dbRole).Error; err != nil {
		panic(err)
	}
	uc.PatchRoleByRoleId(ctx, role, dbRole.Id)
}

// GetRoleOptionMap 获取角色Option列表 {label: Name, value: RoleCode}
func (uc *AdminPermsUseCase) GetRoleOptionMap(ctx context.Context, search string) (options []map[string]any, err error) {
	var roles []*permission.AdminRole
	query := uc.db.WithContext(ctx).Model(&permission.AdminRole{}).Select("role_code, name")
	if search != "" {
		query = query.Where("name LIKE ?", "%"+search+"%")
	}
	if err = query.Find(&roles).Error; err != nil {
		panic(err)
	}
	for _, role := range roles {
		options = append(options, map[string]any{
			"label": role.Name,
			"value": role.RoleCode,
		})
	}
	return
}

func (uc *AdminPermsUseCase) CreateAPI(ctx context.Context, api *permission.AdminAPI) {
	if err := uc.db.WithContext(ctx).Create(&api).Error; err != nil {
		panic(err)
	}
	return
}

func (uc *AdminPermsUseCase) PatchAPIByAPIId(ctx context.Context, api *permission.AdminAPI, apiId int64) {
	if err := uc.db.WithContext(ctx).Updates(&api).Where(apiId).Error; err != nil {
		panic(err)
	}
}

func (uc *AdminPermsUseCase) CreateAPIGroup(ctx context.Context, group *permission.AdminAPIGroup) {
	if err := uc.db.WithContext(ctx).Create(&group).Error; err != nil {
		panic(err)
	}
	return
}

func (uc *AdminPermsUseCase) SetRoleEmployeesByRoleCode(ctx context.Context, employeeIds []int64, roleCode string) error {
	_, err := uc.FindOneRoleByRoleCode(ctx, roleCode)
	if err != nil {
		return err
	}

	accounts := uc.employee.FindAccountsByIds(ctx, employeeIds)
	if len(accounts) == 0 {
		return nil
	}

	var policies [][]string
	for _, account := range accounts {
		policies = append(policies, []string{account, roleCode})
	}

	if _, err := uc.Casbin.RemoveFilteredGroupingPolicy(1, roleCode); err != nil {
		panic(err)
	}
	if _, err := uc.Casbin.AddGroupingPolicies(policies); err != nil {
		panic(err)
	}
	return nil
}

// ReplaceEmployeeRoles Replace 员工角色
func (uc *AdminPermsUseCase) ReplaceEmployeeRoles(ctx context.Context, employeeId int64, roleCodes []string) error {
	employee, err := uc.employee.FindOneEmployeeById(ctx, employeeId)
	if err != nil {
		return err
	}

	var policies [][]string
	for _, roleCode := range roleCodes {
		policies = append(policies, []string{employee.Account, roleCode})
	}

	if _, err := uc.Casbin.RemoveFilteredGroupingPolicy(0, employee.Account); err != nil {
		panic(err)
	}
	if _, err := uc.Casbin.AddGroupingPolicies(policies); err != nil {
		panic(err)
	}
	return nil
}

func (uc *AdminPermsUseCase) PatchAPIGroupByAPIGroupId(ctx context.Context, group *permission.AdminAPIGroup, groupId int64) {
	if err := uc.db.WithContext(ctx).Updates(&group).Where(groupId).Error; err != nil {
		panic(err)
	}
}

func (uc *AdminPermsUseCase) FindAllAPI(ctx context.Context) (apis []*permission.AdminAPI) {
	if err := uc.db.WithContext(ctx).Model(permission.AdminAPI{}).Preload("Group").Find(&apis).Error; err != nil {
		panic(err)
	}
	return
}
