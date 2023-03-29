package powerx

import (
	"PowerX/internal/types"
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
	"os"
	"strings"
)

type AuthUseCase struct {
	db          *gorm.DB
	Casbin      *casbin.Enforcer
	sqlAdapter  *sqladapter.Adapter
	fileAdapter *fileadapter.Adapter
	metadata    *MetadataCtx
	employee    *OrganizationUseCase
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
	types.Model
	API     string
	Method  string
	Name    string
	Desc    string
	GroupId int64
	Group   AdminAPIGroup
}

type AdminAPIGroup struct {
	types.Model
	GroupCode string `gorm:"unique"`
	Prefix    string
	Name      string
	Desc      string
}

type AdminRole struct {
	types.Model
	RoleCode   string `gorm:"unique"`
	Name       string
	Desc       string
	IsReserved bool
	AdminAPI   []*AdminAPI `gorm:"many2many:admin_role_apis"`
	MenuNames  []*AdminRoleMenuName
}

type AdminRoleMenuName struct {
	types.Model
	AdminRoleId int64
	MenuName    string
}

func NewCasbinUseCase(db *gorm.DB, md *MetadataCtx, employee *OrganizationUseCase) *AuthUseCase {
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
	return &AuthUseCase{
		db:          db,
		Casbin:      e,
		sqlAdapter:  a,
		fileAdapter: f,
		metadata:    md,
		employee:    employee,
	}
}

func (a *AuthUseCase) Init() {
	var count int64

	// 初始化API
	if err := a.db.Model(&AdminAPI{}).Count(&count).Error; err != nil {
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

			if err := a.db.Model(&AdminAPIGroup{}).Create(&groups).Error; err != nil {
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
			if err := a.db.Model(&AdminAPIGroup{}).Find(&groups).Error; err != nil {
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

			if err := a.db.Model(&AdminAPI{}).Create(&apis).Error; err != nil {
				panic(errors.Wrap(err, "init api failed"))
			}

		}

		initAPIGroup()
		initAPI()
	}

	// 初始化用户
	if err := a.db.Model(&Employee{}).Count(&count).Error; err != nil {
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
		if err := a.db.Model(&Employee{}).Create(&root).Error; err != nil {
			panic(errors.Wrap(err, "init root failed"))
		}
	}

	// 初始化casbin策略
	if err := a.db.Model(&EmployeeCasbinPolicy{}).Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "init casbin policy failed"))
	}
	if count == 0 {
		a.Casbin.SetAdapter(a.fileAdapter)
		a.Casbin.LoadPolicy()
		a.Casbin.SetAdapter(a.sqlAdapter)
		a.Casbin.SavePolicy()
	}

	// 初始化角色
	if err := a.db.Model(&AdminRole{}).Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "init role failed"))
	}
	if count == 0 {
		var roles []AdminRole

		var apis []*AdminAPI
		if err := a.db.Model(&AdminAPI{}).Find(&apis).Error; err != nil {
			panic(errors.Wrap(err, "init role failed"))
		}

		roles = append(roles, AdminRole{
			RoleCode:   "admin",
			Name:       "管理员",
			Desc:       "管理员",
			AdminAPI:   apis,
			IsReserved: true,
		}, AdminRole{
			RoleCode:   "common_employee",
			Name:       "普通员工",
			Desc:       "普通员工",
			IsReserved: true,
		})
		if err := a.db.Model(&AdminRole{}).Create(&roles).Error; err != nil {
			panic(errors.Wrap(err, "init roles failed"))
		}

		var adminPolicies [][]string
		for _, api := range apis {
			adminPolicies = append(adminPolicies, []string{"admin", api.API, api.Method})
		}
		if _, err := a.Casbin.AddPolicies(adminPolicies); err != nil {
			panic(errors.Wrap(err, "init casbin policy failed"))
		}
	}
}

func (a *AuthUseCase) FindOneRoleByRoleCode(ctx context.Context, roleCode string) (role *AdminRole, err error) {
	err = a.db.WithContext(ctx).Where(AdminRole{RoleCode: roleCode}).
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

func (a *AuthUseCase) FindAllRoles(ctx context.Context) (roles []*AdminRole) {
	if err := a.db.WithContext(ctx).Model(AdminRole{}).Preload("MenuNames").Find(&roles).Error; err != nil {
		panic(err)
	}
	return
}

func (a *AuthUseCase) CreateRole(ctx context.Context, role *AdminRole) {
	if err := a.db.WithContext(ctx).Create(&role).Error; err != nil {
		panic(err)
	}
	return
}

func (a *AuthUseCase) PatchRoleByRoleId(ctx context.Context, role *AdminRole, roleId int64) {
	var dbRole AdminRole
	if err := a.db.Find(&dbRole, roleId).Error; err != nil {
		panic(err)
	}
	a.PatchRoleByRoleCode(ctx, role, dbRole.RoleCode)
}

func (a *AuthUseCase) PatchRoleByRoleCode(ctx context.Context, role *AdminRole, roleCode string) {
	err := a.db.Transaction(func(tx *gorm.DB) error {
		if err := a.db.WithContext(ctx).Updates(&role).Where(AdminRole{RoleCode: roleCode}).Error; err != nil {
			return err
		}
		_, err := a.Casbin.DeleteRole(roleCode)
		if err != nil {
			return err
		}

		apiIds := slicex.SlicePluck(role.AdminAPI, func(item *AdminAPI) int64 {
			return item.ID
		})

		var apis []*AdminAPI
		if err := a.db.Model(&AdminAPI{}).Find(&apis, apiIds).Error; err != nil {
			panic(errors.Wrap(err, "find api failed"))
		}

		var policies [][]string
		for _, api := range apis {
			policies = append(policies, []string{role.RoleCode, api.API, api.Method})
		}

		_, err = a.Casbin.AddPolicies(policies)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		panic(err)
	}
}

func (a *AuthUseCase) CreateAPI(ctx context.Context, api *AdminAPI) {
	if err := a.db.WithContext(ctx).Create(&api).Error; err != nil {
		panic(err)
	}
	return
}

func (a *AuthUseCase) PatchAPIByAPIId(ctx context.Context, api *AdminAPI, apiId int64) {
	if err := a.db.WithContext(ctx).Updates(&api).Where(apiId).Error; err != nil {
		panic(err)
	}
}

func (a *AuthUseCase) CreateAPIGroup(ctx context.Context, group *AdminAPIGroup) {
	if err := a.db.WithContext(ctx).Create(&group).Error; err != nil {
		panic(err)
	}
	return
}

func (a *AuthUseCase) PatchAPIGroupByAPIGroupId(ctx context.Context, group *AdminAPIGroup, groupId int64) {
	if err := a.db.WithContext(ctx).Updates(&group).Where(groupId).Error; err != nil {
		panic(err)
	}
}

func (a *AuthUseCase) FindAllAPI(ctx context.Context) (apis []*AdminAPI) {
	if err := a.db.WithContext(ctx).Model(AdminAPI{}).Find(&apis).Error; err != nil {
		panic(err)
	}
	return
}
