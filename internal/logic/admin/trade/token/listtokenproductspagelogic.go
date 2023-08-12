package token

import (
	product2 "PowerX/internal/logic/admin/product"
	"PowerX/internal/model/product"
	productUC "PowerX/internal/uc/powerx/product"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListTokenProductsPageLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListTokenProductsPageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListTokenProductsPageLogic {
	return &ListTokenProductsPageLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListTokenProductsPageLogic) ListTokenProductsPage(req *types.ListProductsPageRequest) (resp *types.ListProductsPageReply, err error) {
	// 去掉代币的产品
	TokenTypeId := l.svcCtx.PowerX.DataDictionary.GetCachedDDId(l.ctx, product.TypeProductType, product.ProductTypeToken)

	page, err := l.svcCtx.PowerX.Product.FindManyProducts(l.ctx, &productUC.FindManyProductsOption{
		LikeName: req.LikeName,
		Types:    []int{TokenTypeId},
		PageEmbedOption: types.PageEmbedOption{
			PageIndex: req.PageIndex,
			PageSize:  req.PageSize,
		},
	})
	if err != nil {
		return nil, err
	}

	// list
	list := product2.TransformProductsToReply(page.List)
	return &types.ListProductsPageReply{
		List:      list,
		PageIndex: page.PageIndex,
		PageSize:  page.PageSize,
		Total:     page.Total,
	}, nil
}
