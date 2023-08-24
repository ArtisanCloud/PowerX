package order

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ImportOrdersLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewImportOrdersLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ImportOrdersLogic {
	return &ImportOrdersLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ImportOrdersLogic) ImportOrders(req *types.ImportOrdersRequest) (resp *types.ImportOrdersReply, err error) {
	// todo: add your logic here and delete this line

	return
}
