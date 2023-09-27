package pricebook

import (
	product2 "PowerX/internal/uc/powerx/crm/product"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListPriceBooksLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListPriceBooksLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListPriceBooksLogic {
	return &ListPriceBooksLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListPriceBooksLogic) ListPriceBooks(req *types.ListPriceBooksPageRequest) (resp *types.ListPriceBooksPageReply, err error) {
	opt := &product2.FindPriceBookOption{
		PageEmbedOption: types.PageEmbedOption{
			PageIndex: req.PageIndex,
			PageSize:  req.PageSize,
		},
	}

	priceBookPageList := l.svcCtx.PowerX.PriceBook.FindManyPriceBooks(l.ctx, opt)
	resp = &types.ListPriceBooksPageReply{}
	for _, priceBook := range priceBookPageList.List {
		priceBookReply := types.PriceBook{
			Id:          priceBook.Id,
			IsStandard:  priceBook.IsStandard,
			Name:        priceBook.Name,
			Description: priceBook.Description,
			StoreId:     priceBook.StoreId,
			CreatedAt:   priceBook.CreatedAt.String(),
		}
		resp.List = append(resp.List, priceBookReply)
	}
	resp.PageIndex = priceBookPageList.PageIndex
	resp.PageSize = priceBookPageList.PageSize
	resp.Total = priceBookPageList.Total

	return resp, nil

}
