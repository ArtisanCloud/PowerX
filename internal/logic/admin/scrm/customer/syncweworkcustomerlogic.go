package customer

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type SyncWeWorkCustomerLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewSyncWeWorkCustomerLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SyncWeWorkCustomerLogic {
	return &SyncWeWorkCustomerLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *SyncWeWorkCustomerLogic) SyncWeWorkCustomer() (resp *types.SyncWeWorkCustomerReply, err error) {
	// todo: add your logic here and delete this line

	return
}
