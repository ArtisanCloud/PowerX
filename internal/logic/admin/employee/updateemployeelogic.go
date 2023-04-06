package employee

import (
	"PowerX/internal/uc/powerx"
	"context"
	"github.com/pkg/errors"
	"time"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type UpdateEmployeeLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpdateEmployeeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateEmployeeLogic {
	return &UpdateEmployeeLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UpdateEmployeeLogic) UpdateEmployee(req *types.UpdateEmployeeRequest) (resp *types.UpdateEmployeeReply, err error) {
	employee := powerx.Employee{
		Model: types.Model{
			ID: req.Id,
		},
		Name:          req.Name,
		NickName:      req.NickName,
		Desc:          req.Desc,
		Position:      req.Position,
		JobTitle:      req.JobTitle,
		DepartmentId:  req.DepId,
		MobilePhone:   req.MobilePhone,
		Gender:        req.Gender,
		Email:         req.Email,
		ExternalEmail: req.ExternalEmail,
		Avatar:        req.Avatar,
		Password:      req.Password,
		Status:        req.Status,
	}

	if err = employee.HashPassword(); err != nil {
		panic(errors.Wrap(err, "create employee hash password failed"))
	}

	if err := l.svcCtx.PowerX.Organization.UpdateEmployeeById(l.ctx, &employee, req.Id); err != nil {
		return nil, err
	}

	roles, _ := l.svcCtx.PowerX.AdminAuthorization.Casbin.GetRolesForUser(employee.Account)

	return &types.UpdateEmployeeReply{
		Employee: &types.Employee{
			Id:            employee.ID,
			Account:       employee.Account,
			Name:          employee.Name,
			Email:         employee.Email,
			MobilePhone:   employee.MobilePhone,
			Gender:        employee.Gender,
			NickName:      employee.NickName,
			Desc:          employee.Desc,
			Avatar:        employee.Avatar,
			ExternalEmail: employee.ExternalEmail,
			Roles:         roles,
			Position:      employee.Position,
			JobTitle:      employee.JobTitle,
			IsEnabled:     employee.Status == powerx.EmployeeStatusEnabled,
			CreatedAt:     employee.CreatedAt.Format(time.RFC3339),
		},
	}, nil
}
