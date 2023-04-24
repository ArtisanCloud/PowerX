package product

import (
	"PowerX/internal/model/product"
	"PowerX/internal/svc"
	"PowerX/internal/types"
	productUC "PowerX/internal/uc/powerx/product"
	"context"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListProductsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListProductsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListProductsLogic {
	return &ListProductsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListProductsLogic) ListProducts(req *types.ListProductsPageRequest) (resp *types.ListProductsPageReply, err error) {
	page, err := l.svcCtx.PowerX.Product.FindManyProducts(l.ctx, &productUC.FindManyProductsOption{
		PageEmbedOption: types.PageEmbedOption{
			PageIndex: req.PageIndex,
			PageSize:  req.PageSize,
		},
	})
	if err != nil {
		return nil, err
	}

	// list
	list := TransformProductsToProductsReply(page.List)
	return &types.ListProductsPageReply{
		List:      list,
		PageIndex: page.PageIndex,
		PageSize:  page.PageSize,
		Total:     page.Total,
	}, nil

}

func TransformProductsToProductsReply(products []*product.Product) []types.Product {
	productsReply := []types.Product{}
	for _, product := range products {
		productReply := TransformProductToProductReply(product)
		productsReply = append(productsReply, *productReply)

	}
	return productsReply
}
