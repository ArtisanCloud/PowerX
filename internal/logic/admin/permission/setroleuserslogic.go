package permission

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type SetRoleUsersLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewSetRoleUsersLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SetRoleUsersLogic {
	return &SetRoleUsersLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *SetRoleUsersLogic) SetRoleUsers(req *types.SetRoleUsersRequest) (resp *types.SetRoleUsersReply, err error) {
	err = l.svcCtx.PowerX.AdminAuthorization.SetRoleUsersByRoleCode(l.ctx, req.UserIds, req.RoleCode)
	if err != nil {
		return nil, err
	}
	return &types.SetRoleUsersReply{
		Status: "ok",
	}, nil
}
