package employee

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type SyncEmployeesLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewSyncEmployeesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SyncEmployeesLogic {
	return &SyncEmployeesLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *SyncEmployeesLogic) SyncEmployees(req *types.SyncEmployeesRequest) (resp *types.SyncEmployeesReply, err error) {
	// todo: add your logic here and delete this line

	return
}
