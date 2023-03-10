package auth

import (
	"PowerX/internal/uc"
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
	cred, err := l.svcCtx.UC.MetadataCtx.AuthMetadataFromContext(l.ctx)
	if err != nil {
		panic(errors.Wrap(err, "get user metadata failed"))
	}

	employee, err := l.svcCtx.UC.Employee.FindOneEmployee(l.ctx, &uc.FindEmployeeOption{
		Ids: []int64{cred.UID},
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

	return &types.GetUserInfoReply{
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
	}, nil
}
