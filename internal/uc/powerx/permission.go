package powerx

import (
	"PowerX/internal/types"
	"PowerX/internal/types/errorx"
	"context"
	sqladapter "github.com/Blank-Xu/sql-adapter"
	"github.com/casbin/casbin/v2"
	fileadapter "github.com/casbin/casbin/v2/persist/file-adapter"
	"github.com/pkg/errors"
	"gorm.io/gorm"
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
	Name    string
	Desc    string
	GroupId int64
	Group   AdminAPIGroup
}

type AdminAPIGroup struct {
	types.Model
	Name string
	Desc string
}

type AdminRole struct {
	types.Model
	RoleCode   string `gorm:"unique"`
	Name       string
	Desc       string
	IsReserved bool
	MenuNames  []AdminRoleMenuName
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
	// 初始化角色
	if err := a.db.Model(&AdminRole{}).Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "init role failed"))
	}
	if count == 0 {
		var roles []AdminRole
		roles = append(roles, AdminRole{
			RoleCode:   "admin",
			Name:       "管理员",
			Desc:       "管理员",
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
	}

	// todo init api

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
}

func (a *AuthUseCase) FindOneRoleByRoleCode(ctx context.Context, roleCode string) (role *AdminRole, err error) {
	err = a.db.WithContext(ctx).Where(AdminRole{RoleCode: roleCode}).First(role).Error
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
	if err := a.db.WithContext(ctx).Updates(&role).Where(roleId).Error; err != nil {
		panic(err)
	}
}

func (a *AuthUseCase) PatchRoleByRoleCode(ctx context.Context, role *AdminRole, roleCode string) {
	if err := a.db.WithContext(ctx).Updates(&role).Where(AdminRole{RoleCode: roleCode}).Error; err != nil {
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
