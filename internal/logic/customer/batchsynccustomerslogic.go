package customer

import (
	"context"

	"PowerX/internal/svc"
	"github.com/zeromicro/go-zero/core/logx"
)

type BatchSyncCustomersLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewBatchSyncCustomersLogic(ctx context.Context, svcCtx *svc.ServiceContext) *BatchSyncCustomersLogic {
	return &BatchSyncCustomersLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *BatchSyncCustomersLogic) BatchSyncCustomers() (err error) {
	// todo: add your logic here and delete this line

	return
}
