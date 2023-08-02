package employee

import (
	"PowerX/internal/model/origanzation"
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
	employee := origanzation.Employee{
		Account:       req.Account,
		Name:          req.Name,
		NickName:      req.NickName,
		Desc:          req.Desc,
		PositionID:    req.PositionId,
		DepartmentId:  req.DepId,
		MobilePhone:   req.MobilePhone,
		Gender:        req.Gender,
		Email:         req.Email,
		ExternalEmail: req.ExternalEmail,
		Avatar:        req.Avatar,
		Password:      "123456",
		Status:        origanzation.EmployeeStatusEnabled,
	}
	if err = employee.HashPassword(); err != nil {
		panic(errors.Wrap(err, "create employee hash password failed"))
	}
	if err = l.svcCtx.PowerX.Organization.CreateEmployee(l.ctx, &employee); err != nil {
		return nil, err
	}

	// 根据职位更新角色
	if employee.PositionID != 0 {
		codes, err := l.svcCtx.PowerX.Organization.FindEmployeePositionRoleCodes(l.ctx, employee.Id)
		if err != nil {
			panic(err)
		}
		if _, err := l.svcCtx.PowerX.AdminAuthorization.Casbin.DeleteRolesForUser(employee.Account); err != nil {
			panic(err)
		}
		if _, err := l.svcCtx.PowerX.AdminAuthorization.Casbin.AddRolesForUser(employee.Account, codes); err != nil {
			panic(err)
		}
	}

	return &types.CreateEmployeeReply{
		Id: employee.Id,
	}, nil
}
