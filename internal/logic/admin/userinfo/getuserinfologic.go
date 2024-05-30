package userinfo

import (
	"PowerX/internal/model/origanzation"
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
	cred, err := l.svcCtx.PowerX.AdminAuthorization.AuthMetadataFromContext(l.ctx)
	if err != nil {
		panic(errors.Wrap(err, "get user metadata failed"))
	}

	user, err := l.svcCtx.PowerX.Organization.FindOneUserById(l.ctx, cred.UID)
	if err != nil {
		return nil, err
	}

	roles, _ := l.svcCtx.PowerX.AdminAuthorization.Casbin.GetRolesForUser(user.Account)

	resp = &types.GetUserInfoReply{
		Id:            user.Id,
		Account:       user.Account,
		Name:          user.Name,
		Email:         user.Email,
		MobilePhone:   user.MobilePhone,
		Gender:        user.Gender,
		NickName:      user.NickName,
		Desc:          user.NickName,
		Avatar:        user.Avatar,
		ExternalEmail: user.ExternalEmail,
		Roles:         roles,
		JobTitle:      user.JobTitle,
		CreatedAt:     user.CreatedAt.Format(time.RFC3339),
	}
	if user.Position != nil {
		resp.Position = user.Position.Name
	}

	if user.Department != nil {
		resp.DepName = user.Department.Name
	}
	if user.Status == origanzation.UserStatusEnabled {
		resp.IsEnabled = true
	}
	return
}
