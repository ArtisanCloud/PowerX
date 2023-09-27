package pricebookentry

import (
	"PowerX/internal/model/powermodel"
	"PowerX/internal/model/product"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type UpdatePriceBookEntryLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpdatePriceBookEntryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdatePriceBookEntryLogic {
	return &UpdatePriceBookEntryLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UpdatePriceBookEntryLogic) UpdatePriceBookEntry(req *types.UpdatePriceBookEntryRequest) (resp *types.UpdatePriceBookEntryReply, err error) {
	priceBook := &product.PriceBookEntry{
		PowerModel: powermodel.PowerModel{
			Id: req.Id,
		},
		PriceBookId: req.PriceBookId,
		ProductId:   req.ProductId,
		SkuId:       req.SkuId,
		UnitPrice:   req.UnitPrice,
		ListPrice:   req.ListPrice,
		IsActive:    req.IsActive,
	}
	priceBook.UniqueID = priceBook.GetComposedUniqueID()

	priceBook, err = l.svcCtx.PowerX.PriceBookEntry.UpsertPriceBookEntry(l.ctx, priceBook)
	if err != nil {
		return nil, err
	}

	return &types.UpdatePriceBookEntryReply{
		Id: req.Id,
	}, nil
}
