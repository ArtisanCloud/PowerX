package pricebookentry

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeletePriceBookEntryLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeletePriceBookEntryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeletePriceBookEntryLogic {
	return &DeletePriceBookEntryLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DeletePriceBookEntryLogic) DeletePriceBookEntry(req *types.DeletePriceBookEntryRequest) (resp *types.DeletePriceBookEntryReply, err error) {
	// todo: add your logic here and delete this line

	return
}
