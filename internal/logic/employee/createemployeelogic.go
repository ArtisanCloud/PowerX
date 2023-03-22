package employee

import (
	"PowerX/internal/svc"
	"PowerX/internal/types"
	"PowerX/internal/uc/powerx"
	"context"
	"github.com/pkg/errors"
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
	gender := (*powerx.Gender)(req.Gender)
	status := powerx.EmployeeStatusEnable
	// create employee
	employee := powerx.Employee{
		Account:       req.Account,
		Name:          req.Name,
		NickName:      req.NickName,
		Desc:          req.Desc,
		Position:      req.Position,
		DepartmentIds: req.DepIds,
		MobilePhone:   req.MobilePhone,
		Gender:        gender,
		Email:         req.Email,
		ExternalEmail: req.ExternalEmail,
		Avatar:        req.Avatar,
		Password:      "123456",
		Status:        &status,
	}
	err = employee.HashPassword()
	if err != nil {
		panic(errors.Wrap(err, "create employee hash password failed"))
	}
	l.svcCtx.UC.Employee.CreateEmployees(l.ctx, []*powerx.Employee{&employee})

	return &types.CreateEmployeeReply{
		Id: employee.ID,
	}, nil
}
