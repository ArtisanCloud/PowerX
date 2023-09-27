package order

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PutOrderLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPutOrderLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PutOrderLogic {
	return &PutOrderLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PutOrderLogic) PutOrder(req *types.PutOrderRequest) (resp *types.PutOrderReply, err error) {
	// todo: add your logic here and delete this line

	return
}
