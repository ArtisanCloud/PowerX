package employee

import (
	"PowerX/internal/model/scrm/organization"
	"context"
	"time"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetEmployeeLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetEmployeeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetEmployeeLogic {
	return &GetEmployeeLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetEmployeeLogic) GetEmployee(req *types.GetEmployeeRequest) (resp *types.GetEmployeeReply, err error) {
	employee, err := l.svcCtx.PowerX.Organization.FindOneEmployeeById(l.ctx, req.Id)
	if err != nil {
		return nil, err
	}

	roles, _ := l.svcCtx.PowerX.AdminAuthorization.Casbin.GetRolesForUser(employee.Account)

	var dep *types.EmployeeDepartment
	if employee.Department != nil {
		dep = &types.EmployeeDepartment{
			DepId:   employee.Department.Id,
			DepName: employee.Department.Name,
		}
	}

	return &types.GetEmployeeReply{
		Employee: &types.Employee{
			Id:            employee.Id,
			Account:       employee.Account,
			Name:          employee.Name,
			Email:         employee.Email,
			MobilePhone:   employee.MobilePhone,
			Gender:        employee.Gender,
			NickName:      employee.NickName,
			Desc:          employee.NickName,
			Avatar:        employee.Avatar,
			ExternalEmail: employee.ExternalEmail,
			Department:    dep,
			Roles:         roles,
			Position:      employee.Position,
			JobTitle:      employee.JobTitle,
			IsEnabled:     employee.Status == organization.EmployeeStatusEnabled,
			CreatedAt:     employee.CreatedAt.Format(time.RFC3339),
		},
	}, nil
}
