package user

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type SyncUsersLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewSyncUsersLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SyncUsersLogic {
	return &SyncUsersLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (sync *SyncUsersLogic) SyncUsers(req *types.SyncUsersRequest) (resp *types.SyncUsersReply, err error) {
	// todo: add your logic here and delete this line

	return
}
