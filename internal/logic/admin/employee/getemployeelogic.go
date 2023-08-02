package employee

import (
	"PowerX/internal/model/origanzation"
	"PowerX/internal/model/permission"
	"PowerX/pkg/slicex"
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

	vo := types.Employee{
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
		JobTitle:      employee.JobTitle,
		IsEnabled:     employee.Status == origanzation.EmployeeStatusEnabled,
		CreatedAt:     employee.CreatedAt.Format(time.RFC3339),
	}
	if employee.Position != nil {
		codes := slicex.SlicePluck(employee.Position.Roles, func(item *permission.AdminRole) string {
			return item.RoleCode
		})
		vo.Position = &types.Position{
			Id:        employee.Position.Id,
			Name:      employee.Position.Name,
			Desc:      employee.Position.Desc,
			Level:     employee.Position.Level,
			RoleCodes: codes,
		}
		vo.PositionId = employee.Position.Id
	}
	return &types.GetEmployeeReply{
		Employee: &vo,
	}, nil
}
