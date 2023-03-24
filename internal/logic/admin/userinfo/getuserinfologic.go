package userinfo

import (
	"PowerX/internal/uc/powerx"
	"context"
	"github.com/pkg/errors"
	"time"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetUserInfoLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetUserInfoLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetUserInfoLogic {
	return &GetUserInfoLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetUserInfoLogic) GetUserInfo() (resp *types.GetUserInfoReply, err error) {
	cred, err := l.svcCtx.PowerX.MetadataCtx.AuthMetadataFromContext(l.ctx)
	if err != nil {
		panic(errors.Wrap(err, "get user metadata failed"))
	}

	employee, err := l.svcCtx.PowerX.Employee.FindOneEmployeeById(l.ctx, cred.UID)
	if err != nil {
		return nil, err
	}

	roles, _ := l.svcCtx.PowerX.Auth.Casbin.GetRolesForUser(employee.Account)

	resp = &types.GetUserInfoReply{
		Id:            employee.ID,
		Account:       employee.Account,
		Name:          employee.Name,
		Email:         employee.Email,
		MobilePhone:   employee.MobilePhone,
		Gender:        employee.Gender,
		NickName:      employee.NickName,
		Desc:          employee.NickName,
		Avatar:        employee.Avatar,
		ExternalEmail: employee.ExternalEmail,
		Roles:         roles,
		Position:      employee.Position,
		JobTitle:      employee.JobTitle,
		CreatedAt:     employee.CreatedAt.Format(time.RFC3339),
	}
	if employee.Department != nil {
		resp.DepName = employee.Department.Name
	}
	if employee.Status == powerx.EmployeeStatusEnabled {
		resp.IsEnabled = true
	}
	return
}
