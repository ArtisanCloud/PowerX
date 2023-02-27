package employee

import (
	"PowerX/internal/uc"
	"context"
	"fmt"
	"github.com/pkg/errors"

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
	employee := uc.Employee{
		Model:         &types.Model{ID: req.Id},
		Name:          req.Name,
		NickName:      req.NickName,
		Desc:          req.Desc,
		Position:      req.Position,
		DepartmentIds: req.DepIds,
		MobilePhone:   req.MobilePhone,
		Gender:        (*uc.Gender)(req.Gender),
		Email:         req.Email,
		ExternalEmail: req.ExternalEmail,
		Avatar:        req.Avatar,
		Password:      req.Password,
		Status:        (*uc.EmployeeStatus)(req.Status),
	}

	err = employee.HashPassword()
	if err != nil {
		panic(errors.Wrap(err, "update employee hash password failed"))
	}

	l.svcCtx.UC.Employee.UpdateEmployeeById(l.ctx, &employee)
	fmt.Println(employee)

	return &types.UpdateEmployeeReply{}, nil
}
