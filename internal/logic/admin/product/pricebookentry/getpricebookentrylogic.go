package pricebookentry

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetPriceBookEntryLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetPriceBookEntryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetPriceBookEntryLogic {
	return &GetPriceBookEntryLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetPriceBookEntryLogic) GetPriceBookEntry(req *types.GetPriceBookEntryRequest) (resp *types.GetPriceBookEntryReply, err error) {
	// todo: add your logic here and delete this line

	return
}
