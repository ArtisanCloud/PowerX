package employee

import (
	"PowerX/internal/uc/powerx"
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
	employee, err := l.svcCtx.UC.Employee.FindOneEmployee(l.ctx, &powerx.FindManyEmployeeOption{
		Ids: []int64{req.Id},
	})
	if err != nil {
		return nil, err
	}

	gender := int8(0)
	if employee.Gender != nil {
		gender = int8(*employee.Gender)
	}

	isEnabled := false
	if employee.Status != nil {
		isEnabled = *(employee.Status) > 0
	}

	roles, _ := l.svcCtx.UC.Auth.Casbin.GetRolesForUser(employee.Account)

	return &types.GetEmployeeReply{
		Employee: &types.Employee{
			Id:            employee.ID,
			Account:       employee.Account,
			Name:          employee.Name,
			Email:         employee.Email,
			MobilePhone:   employee.MobilePhone,
			Gender:        gender,
			NickName:      employee.NickName,
			Desc:          employee.NickName,
			Avatar:        employee.Avatar,
			ExternalEmail: employee.ExternalEmail,
			DepIds:        employee.DepartmentIds,
			Roles:         roles,
			Position:      employee.Position,
			JobTitle:      employee.JobTitle,
			IsEnabled:     isEnabled,
			CreatedAt:     employee.CreatedAt.Format(time.RFC3339),
		},
	}, nil
}
