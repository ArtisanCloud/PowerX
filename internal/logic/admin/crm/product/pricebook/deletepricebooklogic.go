package pricebook

import (
	"PowerX/internal/types/errorx"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeletePriceBookLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeletePriceBookLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeletePriceBookLogic {
	return &DeletePriceBookLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DeletePriceBookLogic) DeletePriceBook(req *types.DeletePriceBookRequest) (resp *types.DeletePriceBookReply, err error) {

	// 先确认当前要删除记录是否是标准价格手册
	standardPriceBook, _ := l.svcCtx.PowerX.PriceBook.GetStandardPriceBook(l.ctx)
	if standardPriceBook != nil && standardPriceBook.Id == req.Id {
		return nil, errorx.ErrCanNotDeleteStandardPrice
	}

	err = l.svcCtx.PowerX.PriceBook.DeletePriceBook(l.ctx, req.Id)
	if err != nil {
		return nil, err
	}

	return &types.DeletePriceBookReply{
		Id: req.Id,
	}, nil
}
