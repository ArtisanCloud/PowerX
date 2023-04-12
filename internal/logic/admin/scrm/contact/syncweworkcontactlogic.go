package contact

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type SyncWeWorkContactLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewSyncWeWorkContactLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SyncWeWorkContactLogic {
	return &SyncWeWorkContactLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *SyncWeWorkContactLogic) SyncWeWorkContact() (resp *types.SyncWeWorkContactReply, err error) {
	l.svcCtx.PowerX.SCRM.Org.SyncDepartmentsAndEmployees(l.ctx)

	return
}
