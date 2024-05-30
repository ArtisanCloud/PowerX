package permission

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type SetUserRolesLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewSetUserRolesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SetUserRolesLogic {
	return &SetUserRolesLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *SetUserRolesLogic) SetUserRoles(req *types.SetUserRolesRequest) (resp *types.SetUserRolesReply, err error) {
	user, err := l.svcCtx.PowerX.Organization.FindOneUserById(l.ctx, req.UserId)
	if err != nil {
		return nil, err
	}

	_, err = l.svcCtx.PowerX.AdminAuthorization.Casbin.DeleteRolesForUser(user.Account)
	if err != nil {
		return nil, err
	}

	_, err = l.svcCtx.PowerX.AdminAuthorization.Casbin.AddRolesForUser(user.Account, req.RoleCodes)
	if err != nil {
		return nil, err
	}

	return &types.SetUserRolesReply{
		Status: "ok",
	}, nil
}
