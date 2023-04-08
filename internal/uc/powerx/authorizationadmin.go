package powerx

import (
	"PowerX/internal/model"
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
	"strings"
)

type AdminPermsUseCase struct {
	db          *gorm.DB
	Casbin      *casbin.Enforcer
	sqlAdapter  *sqladapter.Adapter
	fileAdapter *fileadapter.Adapter
	employee    *OrganizationUseCase
}

func NewAdminPermsUseCase(db *gorm.DB, employee *OrganizationUseCase) *AdminPermsUseCase {
	//casbin适配器
	sqlDB, _ := db.DB()
	a, err := sqladapter.NewAdapter(sqlDB, "postgres", "casbin_policies")
	if err != nil {
		panic(err)
	}
	f := fileadapter.NewAdapter("etc/rbac_policy.csv")
	e, err := casbin.NewEnforcer("etc/rbac_model.conf", a)
	if err != nil {
		panic(err)
	}
	return &AdminPermsUseCase{
		db:          db,
		Casbin:      e,
		sqlAdapter:  a,
		fileAdapter: f,
		employee:    employee,
	}
}

type adminAuthMetadataKey struct{}

type AdminAuthMetadata struct {
	UID int64
}

func (uc *AdminPermsUseCase) WithAuthMetadataCtxValue(ctx context.Context, md *AdminAuthMetadata) context.Context {
	return context.WithValue(ctx, adminAuthMetadataKey{}, md)
}

func (uc *AdminPermsUseCase) AuthMetadataFromContext(ctx context.Context) (*AdminAuthMetadata, error) {
	v, ok := ctx.Value(adminAuthMetadataKey{}).(*AdminAuthMetadata)
	if !ok {
		return nil, errors.New("无法获取AuthMetadata")
	}
	return v, nil
}

type EmployeeCasbinPolicy struct {
	ID    int64 `gorm:"primarykey"`
	PType string
	V0    string
	V1    string
	V2    string
	V3    string
	V4    string
	V5    string
}

type AdminAPI struct {
	model.Model
	API     string
	Method  string
	Name    string
	Desc    string
	GroupId int64
	Group   AdminAPIGroup
}

type AdminAPIGroup struct {
	model.Model
	GroupCode string `gorm:"unique"`
	Prefix    string
	Name      string
	Desc      string
}

type AdminRole struct {
	model.Model
	RoleCode   string `gorm:"unique"`
	Name       string
	Desc       string
	IsReserved bool
	AdminAPI   []*AdminAPI `gorm:"many2many:admin_role_apis"`
	MenuNames  []*AdminRoleMenuName
}

type AdminRoleMenuName struct {
	model.Model
	AdminRoleId int64
	MenuName    string
}

func (uc *AdminPermsUseCase) Init() {
	var count int64

	// 初始化API
	if err := uc.db.Model(&AdminAPI{}).Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "init api failed"))
	}
	if count == 0 {
		// api group
		initAPIGroup := func() {
			file, err := os.Open("etc/admin_api_group.csv")
			if err != nil {
				panic(err)
			}
			csvReader := csv.NewReader(file)
			records, err := csvReader.ReadAll()
			if err != nil {
				panic(err)
			}

			var groups []AdminAPIGroup
			for _, record := range records {
				groups = append(groups, AdminAPIGroup{
					GroupCode: record[0],
					Prefix:    record[1],
					Name:      record[2],
					Desc:      record[3],
				})
			}

			if err := uc.db.Model(&AdminAPIGroup{}).Create(&groups).Error; err != nil {
				panic(errors.Wrap(err, "init api group failed"))
			}
		}

		// api
		initAPI := func() {
			file, err := os.Open("etc/admin_api.csv")
			if err != nil {
				panic(err)
			}
			csvReader := csv.NewReader(file)
			records, err := csvReader.ReadAll()
			if err != nil {
				panic(err)
			}

			var groups []*AdminAPIGroup
			if err := uc.db.Model(&AdminAPIGroup{}).Find(&groups).Error; err != nil {
				panic(errors.Wrap(err, "init api failed"))
			}

			groupMap := mapx.MapByFunc(groups, func(item *AdminAPIGroup) (string, int64) {
				return item.GroupCode, item.ID
			})

			var apis []AdminAPI
			for _, record := range records {
				var groupId int64
				if id, ok := groupMap[record[0]]; ok {
					groupId = id
				}
				apis = append(apis, AdminAPI{
					API:     record[1],
					Method:  strings.ToUpper(record[2]),
					Name:    record[3],
					GroupId: groupId,
				})
			}

			if err := uc.db.Model(&AdminAPI{}).Create(&apis).Error; err != nil {
				panic(errors.Wrap(err, "init api failed"))
			}

		}

		initAPIGroup()
		initAPI()
	}

	// 初始化用户
	if err := uc.db.Model(&Employee{}).Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "init role failed"))
	}
	if count == 0 {
		root := Employee{
			Account:    "root",
			Password:   "root",
			Name:       "超级管理员",
			Status:     EmployeeStatusEnabled,
			IsReserved: true,
		}
		root.HashPassword()
		if err := uc.db.Model(&Employee{}).Create(&root).Error; err != nil {
			panic(errors.Wrap(err, "init root failed"))
		}
	}

	// 初始化casbin策略
	if err := uc.db.Model(&EmployeeCasbinPolicy{}).Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "init casbin policy failed"))
	}
	if count == 0 {
		uc.Casbin.SetAdapter(uc.fileAdapter)
		uc.Casbin.LoadPolicy()
		uc.Casbin.SetAdapter(uc.sqlAdapter)
		uc.Casbin.SavePolicy()
	}

	// 初始化角色
	if err := uc.db.Model(&AdminRole{}).Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "init role failed"))
	}
	if count == 0 {
		var roles []*AdminRole

		var apis []*AdminAPI
		if err := uc.db.Model(&AdminAPI{}).Find(&apis).Error; err != nil {
			panic(errors.Wrap(err, "init role failed"))
		}

		roles = append(roles, &AdminRole{
			RoleCode:   "admin",
			Name:       "管理员",
			Desc:       "管理员",
			AdminAPI:   apis,
			IsReserved: true,
		}, &AdminRole{
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

func (uc *AdminPermsUseCase) FindOneRoleByRoleCode(ctx context.Context, roleCode string) (role *AdminRole, err error) {
	err = uc.db.WithContext(ctx).Where(AdminRole{RoleCode: roleCode}).
		Preload("AdminAPI").
		Preload("MenuNames").
		First(&role).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未查找到角色")
		}
		panic(err)
	}
	return
}

func (uc *AdminPermsUseCase) FindAllRoles(ctx context.Context) (roles []*AdminRole) {
	if err := uc.db.WithContext(ctx).Model(AdminRole{}).Preload("MenuNames").Find(&roles).Error; err != nil {
		panic(err)
	}
	return
}

// CreateRole 创建角色
func (uc *AdminPermsUseCase) CreateRole(ctx context.Context, role *AdminRole) error {
	var count int64
	if uc.db.Model(&AdminRole{}).Where(AdminRole{RoleCode: role.RoleCode}).Count(&count); count > 0 {
		return errorx.WithCause(errorx.ErrBadRequest, "角色已存在")
	}

	err := uc.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		tx.Model(&AdminRole{}).Create(role)

		if len(role.AdminAPI) == 0 {
			return nil
		}

		apiIds := slicex.SlicePluck(role.AdminAPI, func(item *AdminAPI) int64 {
			return item.ID
		})

		var apis []*AdminAPI
		if err := tx.Model(&AdminAPI{}).Where(apiIds).Find(&apis).Error; err != nil {
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
func (uc *AdminPermsUseCase) PatchRoleByRoleId(ctx context.Context, role *AdminRole, roleId int64) {
	role.ID = roleId

	err := uc.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 清除旧的关联关系
		if err := tx.Model(&role).Where(&AdminRole{RoleCode: role.RoleCode}).
			Association("AdminAPI").Clear(); err != nil {
			return err
		}

		// 更新角色信息
		if err := tx.Omit("AdminAPI.*").Where(roleId).Clauses(clause.Returning{}).
			Updates(&role).Error; err != nil {
			return err
		}

		// 如果没有关联的 AdminAPI，返回
		if len(role.AdminAPI) == 0 {
			return nil
		}

		// 删除旧的权限策略
		if _, err := uc.Casbin.DeleteRole(role.RoleCode); err != nil {
			return err
		}

		apiIds := slicex.SlicePluck(role.AdminAPI, func(item *AdminAPI) int64 {
			return item.ID
		})

		var apis []*AdminAPI
		if err := tx.Model(&AdminAPI{}).Where(apiIds).Find(&apis).Error; err != nil {
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

		return nil
	})

	if err != nil {
		panic(err)
	}
}

func (uc *AdminPermsUseCase) PatchRoleByRoleCode(ctx context.Context, role *AdminRole, roleCode string) {
	var dbRole AdminRole
	if err := uc.db.Where(&AdminRole{RoleCode: roleCode}).Find(&dbRole).Error; err != nil {
		panic(err)
	}
	uc.PatchRoleByRoleId(ctx, role, dbRole.ID)
}

func (uc *AdminPermsUseCase) CreateAPI(ctx context.Context, api *AdminAPI) {
	if err := uc.db.WithContext(ctx).Create(&api).Error; err != nil {
		panic(err)
	}
	return
}

func (uc *AdminPermsUseCase) PatchAPIByAPIId(ctx context.Context, api *AdminAPI, apiId int64) {
	if err := uc.db.WithContext(ctx).Updates(&api).Where(apiId).Error; err != nil {
		panic(err)
	}
}

func (uc *AdminPermsUseCase) CreateAPIGroup(ctx context.Context, group *AdminAPIGroup) {
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

func (uc *AdminPermsUseCase) PatchAPIGroupByAPIGroupId(ctx context.Context, group *AdminAPIGroup, groupId int64) {
	if err := uc.db.WithContext(ctx).Updates(&group).Where(groupId).Error; err != nil {
		panic(err)
	}
}

func (uc *AdminPermsUseCase) FindAllAPI(ctx context.Context) (apis []*AdminAPI) {
	if err := uc.db.WithContext(ctx).Model(AdminAPI{}).Preload("Group").Find(&apis).Error; err != nil {
		panic(err)
	}
	return
}
