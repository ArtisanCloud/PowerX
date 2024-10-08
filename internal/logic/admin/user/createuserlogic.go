package user

import (
	"context"
	"github.com/pkg/errors"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateUserLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateUserLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateUserLogic {
	return &CreateUserLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateUserLogic) CreateUser(req *types.CreateUserRequest) (resp *types.CreateUserReply, err error) {
	user := organization.User{
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
		Status:        organization.UserStatusEnabled,
	}
	if err = user.HashPassword(); err != nil {
		panic(errors.Wrap(err, "create user hash password failed"))
	}
	if err = l.svcCtx.PowerX.Organization.CreateUser(l.ctx, &user); err != nil {
		return nil, err
	}

	// 根据职位更新角色
	if user.PositionID != 0 {
		codes, err := l.svcCtx.PowerX.Organization.FindUserPositionRoleCodes(l.ctx, user.Id)
		if err != nil {
			panic(err)
		}
		if _, err := l.svcCtx.PowerX.AdminAuthorization.Casbin.DeleteRolesForUser(user.Account); err != nil {
			panic(err)
		}
		if _, err := l.svcCtx.PowerX.AdminAuthorization.Casbin.AddRolesForUser(user.Account, codes); err != nil {
			panic(err)
		}
	}

	return &types.CreateUserReply{
		Id: user.Id,
	}, nil
}
