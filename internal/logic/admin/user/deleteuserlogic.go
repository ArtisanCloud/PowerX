package user

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeleteUserLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeleteUserLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteUserLogic {
	return &DeleteUserLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DeleteUserLogic) DeleteUser(req *types.DeleteUserRequest) (resp *types.DeleteUserReply, err error) {
	err = l.svcCtx.PowerX.Organization.DeleteUserByUuid(l.ctx, req.Uuid)
	if err != nil {
		return nil, err
	}
	return &types.DeleteUserReply{
		Uuid: req.Uuid,
	}, nil
}
