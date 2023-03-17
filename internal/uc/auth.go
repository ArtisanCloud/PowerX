package uc

import (
	"PowerX/internal/types"
	"PowerX/internal/types/errorx"
	"context"
	"encoding/csv"
	"fmt"
	sqladapter "github.com/Blank-Xu/sql-adapter"
	"github.com/casbin/casbin/v2"
	fileadapter "github.com/casbin/casbin/v2/persist/file-adapter"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/pkg/errors"
	"github.com/zeromicro/go-zero/core/logx"
	"gorm.io/gorm"
	"os"
	"path"
	"strings"
	"time"
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

type AuthRestAction struct {
	ResCode      string `gorm:"index"`
	Version      string
	RestPath     string
	Action       string
	FullRestPath string
	Key          string
	Desc         string
	*types.Model
}

func (a *AuthRestAction) AutoSetField() *AuthRestAction {
	a.FullRestPath = FormatAuthRestObj(a.ResCode, a.Version, a.RestPath)
	a.Key = fmt.Sprintf("%s_%s", a.FullRestPath, a.Action)
	return a
}

type AuthRole struct {
	RoleCode   string `gorm:"unique"`
	Name       string
	Desc       string
	IsReserved bool
	MenuNames  pq.StringArray `gorm:"type:text[]"`
	*types.Model
}

/* model */

type AuthResWithAct struct {
	*AuthRecourse `gorm:"embedded"`
	Acts          []*AuthRestAction `gorm:"foreignKey:ResCode;references:ResCode"`
}

func FormatAuthRestObj(resCode string, version string, restPath string, params ...string) string {
	ss := strings.Split(restPath, "/")
	for i, item := range ss {
		ss[i] = item
		if !strings.HasPrefix(item, ":") {
			continue
		}
		if len(params) == 0 {
			continue
		}
		ss[i] = params[0]
	}
	return fmt.Sprintf("/api/%s/%s/", resCode, version) + path.Join(ss...)
}

func FormatRestAction(actions ...string) string {
	var b strings.Builder
	for i, s := range actions {
		b.WriteString("(")
		b.WriteString(s)
		b.WriteString(")")
		if i != 0 && i != len(actions)-1 {
			b.WriteString("|")
		}
	}
	return b.String()
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
	if count == 0 {
		do1 := func() {
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
			for _, row := range rows {
				if len(row) == 0 {
					continue
				}
				if row[0] == "r" && len(row) == 5 {
					resSlice = append(resSlice, &AuthRecourse{
						ResCode: strings.TrimSpace(row[1]),
						ResName: strings.TrimSpace(row[2]),
						Type:    strings.TrimSpace(row[3]),
						Desc:    strings.TrimSpace(row[4]),
					})
				}
			}
			if len(resSlice) > 0 {
				if err := a.db.Model(&AuthRecourse{}).Create(&resSlice).Error; err != nil {
					panic(errors.Wrap(err, "init auth res failed"))
				}
			}
		}

		do2 := func() {
			file, err := os.OpenFile("etc/rbac_action.csv", os.O_RDONLY, 0644)
			if err != nil {
				logx.Error(err)
				return
			}
			rows, err := csv.NewReader(file).ReadAll()
			if err != nil {
				logx.Error(err)
				return
			}
			var actSlice []*AuthRestAction
			for _, row := range rows {
				if len(row) == 0 {
					continue
				}
				if row[0] == "a" && len(row) == 6 {
					act := AuthRestAction{
						ResCode:  strings.TrimSpace(row[1]),
						Version:  strings.TrimSpace(row[2]),
						RestPath: strings.TrimSpace(row[3]),
						Action:   strings.TrimSpace(row[4]),
						Desc:     strings.TrimSpace(row[5]),
					}
					actSlice = append(actSlice, act.AutoSetField())
				}
			}
			if len(actSlice) > 0 {
				if err := a.db.Model(&AuthRestAction{}).Create(&actSlice).Error; err != nil {
					panic(errors.Wrap(err, "init auth act failed"))
				}
			}
		}

		do1()
		do2()
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
		root.HashPassword()
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
		res.Acts = make([]*AuthRestAction, 0)
		resMap[res.ResCode] = res
	}
	var acts []*AuthRestAction
	if err := a.db.WithContext(ctx).Model(&AuthRestAction{}).Find(&acts).Error; err != nil {
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

func (a *AuthUseCase) FindManyAuthRolesByIds(ctx context.Context, ids []int64) []*AuthRole {
	var roles []*AuthRole
	if err := a.db.WithContext(ctx).Model(&AuthRole{}).Where("id in ?", ids).Find(&roles).Error; err != nil {
		panic(errors.Wrap(err, "find roles failed"))
	}
	return roles
}

func (a *AuthUseCase) FindOneAuthRoleById(ctx context.Context, id int64) (*AuthRole, error) {
	var role AuthRole
	if err := a.db.WithContext(ctx).Model(&AuthRole{}).Where(id).Find(&role).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未查找到角色")
		}
		panic(errors.Wrap(err, "find roles failed"))
	}
	return &role, nil
}

func (a *AuthUseCase) FindOneAuthRoleByCode(ctx context.Context, roleCode string) (*AuthRole, error) {
	var role AuthRole
	if err := a.db.WithContext(ctx).Where(AuthRole{RoleCode: roleCode}).Find(&role).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未查找到角色")
		}
		panic(errors.Wrap(err, "find roles failed"))
	}
	return &role, nil
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

func (a *AuthUseCase) FindManyRestActsByIds(ctx context.Context, ids []int64) []*AuthRestAction {
	var acts []*AuthRestAction
	err := a.db.WithContext(ctx).Model(&AuthRestAction{}).Where("id in ?", ids).Find(&acts).Error
	if err != nil {
		panic(errors.Wrap(err, "find acts failed"))
	}
	return acts
}

func (a *AuthUseCase) FindManyRestActsByKeys(ctx context.Context, keys []string) []*AuthRestAction {
	var acts []*AuthRestAction
	err := a.db.WithContext(ctx).Model(&AuthRestAction{}).Where("key in ?", keys).Find(&acts).Error
	if err != nil {
		panic(errors.Wrap(err, "find acts failed"))
	}
	return acts
}

func (a *AuthUseCase) PatchRoleByRoleCode(ctx context.Context, roleCode string, role *AuthRole) {
	err := a.db.WithContext(ctx).Where(AuthRole{RoleCode: roleCode}).Updates(role).Error
	if err != nil {
		panic(errors.Wrap(err, "update role failed"))
	}
}

type OpenAppCertificate struct {
	AppId       string
	Secret      string
	AccessToken string
	ExpiredAt   time.Time
	*types.Model
}

const defaultExpired = time.Hour * 24 * 3

func (a *AuthUseCase) SignOpenAppAccessToken(ctx context.Context, appId string, secret string) (accessToken string, err error) {
	var cert OpenAppCertificate
	err = a.db.WithContext(ctx).Find(OpenAppCertificate{AppId: appId}).Scan(&cert).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", errorx.WithCause(errorx.ErrBadRequest, "AppId不存在或Secret错误")
		}
		panic(errors.Wrap(err, "find open app by app id failed"))
	}
	if cert.Secret != secret {
		return "", errorx.WithCause(errorx.ErrBadRequest, "AppId不存在或Secret错误")
	}
	cert.AccessToken = uuid.New().String()
}
