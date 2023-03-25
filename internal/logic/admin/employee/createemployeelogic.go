package employee

import (
	"PowerX/internal/uc/powerx"
	"context"
	"github.com/pkg/errors"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateEmployeeLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateEmployeeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateEmployeeLogic {
	return &CreateEmployeeLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateEmployeeLogic) CreateEmployee(req *types.CreateEmployeeRequest) (resp *types.CreateEmployeeReply, err error) {
	employee := powerx.Employee{
		Account:       req.Account,
		Name:          req.Name,
		NickName:      req.NickName,
		Desc:          req.Desc,
		Position:      req.Position,
		DepartmentId:  req.DepId,
		MobilePhone:   req.MobilePhone,
		Gender:        req.Gender,
		Email:         req.Email,
		ExternalEmail: req.ExternalEmail,
		Avatar:        req.Avatar,
		Password:      "123456",
		Status:        powerx.EmployeeStatusEnabled,
	}
	err = employee.HashPassword()
	if err != nil {
		panic(errors.Wrap(err, "create employee hash password failed"))
	}
	l.svcCtx.PowerX.Organization.CreateEmployee(l.ctx, &employee)

	return &types.CreateEmployeeReply{
		Id: employee.ID,
	}, nil
}
