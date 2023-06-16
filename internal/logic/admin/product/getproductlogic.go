package product

import (
	"PowerX/internal/model/product"
	"PowerX/internal/types/errorx"
	"context"
	"math"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetProductLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetProductLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetProductLogic {
	return &GetProductLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetProductLogic) GetProduct(req *types.GetProductRequest) (resp *types.GetProductReply, err error) {
	mdlProduct, err := l.svcCtx.PowerX.Product.GetProduct(l.ctx, req.ProductId)

	if err != nil {
		return nil, errorx.ErrNotFoundObject
	}

	return &types.GetProductReply{
		Product: TransformProductToProductReply(mdlProduct),
	}, nil

}

func TransformPriceEntryToPriceEntryReply(entries []*product.PriceBookEntry) (entriesReply *types.PriceEntry) {
	//fmt.Dump(entries)
	for _, entry := range entries {
		if entry.SkuId == 0 && entry.IsActive {
			discount := (entry.UnitPrice / entry.ListPrice) * 100
			discount = math.Round(discount*10) / 10 // 四舍五入保留一位小数

			return &types.PriceEntry{
				Id:        entry.Id,
				UnitPrice: entry.UnitPrice,
				ListPrice: entry.ListPrice,
				Discount:  discount,
			}
		}
	}

	return nil
}
