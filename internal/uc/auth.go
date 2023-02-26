package uc

import (
	"PowerX/internal/types"
	"context"
	"encoding/csv"
	"fmt"
	sqladapter "github.com/Blank-Xu/sql-adapter"
	"github.com/casbin/casbin/v2"
	fileadapter "github.com/casbin/casbin/v2/persist/file-adapter"
	"github.com/pkg/errors"
	"github.com/zeromicro/go-zero/core/logx"
	"gorm.io/gorm"
	"os"
)

type AuthUseCase struct {
	db          *gorm.DB
	Casbin      *casbin.Enforcer
	sqlAdapter  *sqladapter.Adapter
	fileAdapter *fileadapter.Adapter
	metadata    *MetadataCtx
	employee    *EmployeeUseCase
}

type CasbinPolicy struct {
	ID    int64 `gorm:"primarykey"`
	PType string
	V0    string
	V1    string
	V2    string
	V3    string
	V4    string
	V5    string
}

type AuthRecourse struct {
	ResCode string `gorm:"unique"`
	ResName string
	Type    string
	Desc    string
	*types.Model
}

type AuthRecourseAction struct {
	ResCode string `gorm:"index"`
	Scope   string
	Action  string
	Desc    string
	*types.Model
}

type AuthRole struct {
	RoleCode   string `gorm:"unique"`
	Name       string
	Desc       string
	IsReserved bool
	*types.Model
}

/* model */

type AuthResWithAct struct {
	*AuthRecourse `gorm:"embedded"`
	Acts          []*AuthRecourseAction `gorm:"foreignKey:ResCode;references:ResCode"`
}

func newCasbinUseCase(db *gorm.DB, md *MetadataCtx, employee *EmployeeUseCase) *AuthUseCase {
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
	if err := a.db.Model(&AuthRole{}).Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "init role failed"))
	}
	if count == 0 {
		var roles []AuthRole
		roles = append(roles, AuthRole{
			RoleCode:   "admin",
			Name:       "管理员",
			Desc:       "管理员",
			IsReserved: true,
		}, AuthRole{
			RoleCode:   "common_employee",
			Name:       "普通员工",
			Desc:       "普通员工",
			IsReserved: true,
		})
		if err := a.db.Model(&AuthRole{}).Create(&roles).Error; err != nil {
			panic(errors.Wrap(err, "init roles failed"))
		}
	}
	// 初始化资源
	if err := a.db.Model(&AuthRecourse{}).Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "init res failed"))
	}
	fmt.Println(count)
	if count == 0 {
		do := func() {
			file, err := os.OpenFile("etc/rbac_res.csv", os.O_RDONLY, 0644)
			if err != nil {
				logx.Error(err)
				return
			}
			rows, err := csv.NewReader(file).ReadAll()
			if err != nil {
				logx.Error(err)
				return
			}
			var resSlice []*AuthRecourse
			var actSlice []*AuthRecourseAction
			for _, row := range rows {
				if len(row) == 0 {
					continue
				}
				if row[0] == "r" && len(row) == 5 {
					resSlice = append(resSlice, &AuthRecourse{
						ResCode: row[1],
						ResName: row[2],
						Type:    row[3],
						Desc:    row[4],
					})
				}
				if row[0] == "a" && len(row) == 5 {
					actSlice = append(actSlice, &AuthRecourseAction{
						ResCode: row[1],
						Scope:   row[2],
						Action:  row[3],
						Desc:    row[4],
					})
				}
			}
			if len(resSlice) > 0 {
				if err := a.db.Model(&AuthRecourse{}).Create(&resSlice).Error; err != nil {
					panic(errors.Wrap(err, "init auth res failed"))
				}
			}
			if len(actSlice) > 0 {
				if err := a.db.Model(&AuthRecourseAction{}).Create(&actSlice).Error; err != nil {
					panic(errors.Wrap(err, "init auth act failed"))
				}
			}
		}

		do()
	}

	// 初始化用户
	if err := a.db.Model(&Employee{}).Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "init role failed"))
	}
	if count == 0 {
		status := EmployeeStatusEnable
		root := Employee{
			Account:    "root",
			Password:   "root",
			Name:       "超级管理员",
			Status:     &status,
			IsReserved: true,
		}
		if err := a.db.Model(&Employee{}).Create(&root).Error; err != nil {
			panic(errors.Wrap(err, "init root failed"))
		}
	}

	// 初始化casbin策略
	if err := a.db.Model(&CasbinPolicy{}).Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "init casbin policy failed"))
	}
	if count == 0 {
		a.Casbin.SetAdapter(a.fileAdapter)
		a.Casbin.LoadPolicy()
		a.Casbin.SetAdapter(a.sqlAdapter)
		a.Casbin.SavePolicy()
	}
}

func (a *AuthUseCase) FindManyAuthResWithActs(ctx context.Context) []*AuthResWithAct {
	var resSlices []*AuthResWithAct
	if err := a.db.WithContext(ctx).Model(&AuthRecourse{}).Find(&resSlices).Error; err != nil {
		panic(errors.Wrap(err, "find res failed"))
	}
	resMap := make(map[string]*AuthResWithAct)
	for _, res := range resSlices {
		resMap[res.ResCode] = res
	}
	var acts []*AuthRecourseAction
	if err := a.db.WithContext(ctx).Model(&AuthRecourseAction{}).Find(&acts).Error; err != nil {
		panic(errors.Wrap(err, "find res acts failed"))
	}
	for _, act := range acts {
		if res, ok := resMap[act.ResCode]; ok {
			res.Acts = append(res.Acts, act)
		}
	}
	return resSlices
}

func (a *AuthUseCase) FindManyAuthRoles(ctx context.Context) []*AuthRole {
	var roles []*AuthRole
	if err := a.db.WithContext(ctx).Model(&AuthRole{}).Find(&roles).Error; err != nil {
		panic(errors.Wrap(err, "find roles failed"))
	}
	return roles
}

func (a *AuthUseCase) AddRolesForEmployee(ctx context.Context, account string, roleCodes []string) {
	_, err := a.Casbin.AddRolesForUser(account, roleCodes)
	if err != nil {
		panic(errors.Wrap(err, "add roles failed"))
	}
}

func (a *AuthUseCase) UpdateRolesForEmployee(ctx context.Context, account string, roleCodes []string) {
	_, err := a.Casbin.DeleteRolesForUser(account)
	if err != nil {
		panic(errors.Wrap(err, "delete roles failed"))
	}
	_, err = a.Casbin.AddRolesForUser(account, roleCodes)
	if err != nil {
		panic(errors.Wrap(err, "update roles failed"))
	}
}
