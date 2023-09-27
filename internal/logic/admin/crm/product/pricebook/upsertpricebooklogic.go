package pricebook

import (
	"PowerX/internal/model/powermodel"
	"PowerX/internal/model/product"
	"PowerX/internal/svc"
	"PowerX/internal/types"
	"PowerX/internal/types/errorx"
	"context"

	"github.com/zeromicro/go-zero/core/logx"
)

type UpsertPriceBookLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpsertPriceBookLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpsertPriceBookLogic {
	return &UpsertPriceBookLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UpsertPriceBookLogic) UpsertPriceBook(req *types.UpsertPriceBookRequest) (resp *types.UpsertPriceBookReply, err error) {

	// 检查是否当前需要新更的价格手册，是否是标准价格手册
	if req.IsStandard {
		standardPriceBook, _ := l.svcCtx.PowerX.PriceBook.GetStandardPriceBook(l.ctx)
		if standardPriceBook != nil && standardPriceBook.Id != req.Id {
			return nil, errorx.ErrOneStandardPriceBookOnly
		}
	}

	priceBook := &product.PriceBook{
		PowerModel: powermodel.PowerModel{
			Id: req.Id,
		},
		IsStandard:  req.IsStandard,
		Name:        req.Name,
		Description: req.Description,
		StoreId:     req.StoreId,
	}

	priceBook, err = l.svcCtx.PowerX.PriceBook.UpsertPriceBook(l.ctx, priceBook)
	if err != nil {
		return nil, err
	}

	return &types.UpsertPriceBookReply{
		PriceBook: &types.PriceBook{
			Id:          priceBook.Id,
			IsStandard:  priceBook.IsStandard,
			Name:        priceBook.Name,
			Description: priceBook.Description,
			StoreId:     priceBook.StoreId,
			CreatedAt:   priceBook.CreatedAt.String(),
		},
	}, nil

}
