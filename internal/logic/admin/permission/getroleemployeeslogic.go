package permission

import (
	"PowerX/internal/uc/powerx"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetRoleEmployeesLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetRoleEmployeesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetRoleEmployeesLogic {
	return &GetRoleEmployeesLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetRoleEmployeesLogic) GetRoleEmployees(req *types.GetRoleEmployeesReqeust) (resp *types.GetRoleEmployeesReply, err error) {
	accounts, _ := l.svcCtx.PowerX.AdminAuthorization.Casbin.GetUsersForRole(req.RoleCode)

	employeePage := l.svcCtx.PowerX.Organization.FindManyEmployeesPage(l.ctx, &powerx.FindManyEmployeesOption{
		Accounts:  accounts,
		PageIndex: req.PageIndex,
		PageSize:  req.PageSize,
	})

	resp = &types.GetRoleEmployeesReply{
		PageIndex: employeePage.PageIndex,
		PageSize:  employeePage.PageSize,
		Total:     employeePage.Total,
	}

	var list []types.RoleEmployee
	for _, employee := range employeePage.List {
		var dep *types.RoleEmployeeDepartment
		if employee.Department != nil {
			dep = &types.RoleEmployeeDepartment{
				Id:   employee.Department.ID,
				Name: employee.Department.Name,
			}
		}
		list = append(list, types.RoleEmployee{
			Id:          employee.ID,
			Name:        employee.Name,
			Nickname:    employee.NickName,
			Account:     employee.Account,
			PhoneNumber: employee.MobilePhone,
			Department:  dep,
			Email:       employee.Email,
		})
	}

	return
}
