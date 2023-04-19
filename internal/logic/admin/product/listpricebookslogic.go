package product

import (
	"PowerX/internal/model/product"
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

func (l *ListPriceBooksLogic) ListPriceBooks(req *types.ListPriceBooksRequest) (resp *types.ListPriceBooksReply, err error) {
	opt := &product.FindPriceBookOption{

		PageIndex: req.PageIndex,
		PageSize:  req.PageSize,
	}

	priceBookPageList := l.svcCtx.PowerX.PriceBook.FindManyPriceBooks(l.ctx, opt)
	resp = &types.ListPriceBooksReply{}
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
