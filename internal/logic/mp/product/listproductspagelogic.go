package product

import (
	product2 "PowerX/internal/model/product"
	productUC "PowerX/internal/uc/powerx/product"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListProductsPageLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListProductsPageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListProductsPageLogic {
	return &ListProductsPageLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListProductsPageLogic) ListProductsPage(req *types.ListProductsPageRequest) (resp *types.ListProductsPageReply, err error) {
	if req.ProductCategoryId <= 0 {
		return &types.ListProductsPageReply{
			List:      nil,
			PageIndex: 0,
			PageSize:  0,
			Total:     0,
		}, nil
	}

	page, err := l.svcCtx.PowerX.Product.FindManyProducts(l.ctx, &productUC.FindManyProductsOption{
		CategoryId:    req.ProductCategoryId,
		NeedActivated: true,
		//OrderBy:       "sort desc",
		PageEmbedOption: types.PageEmbedOption{
			PageIndex: req.PageIndex,
			PageSize:  req.PageSize,
		},
	})
	if err != nil {
		return nil, err
	}

	// list
	list := TransformProductsToReplyForMP(page.List)
	return &types.ListProductsPageReply{
		List:      list,
		PageIndex: page.PageIndex,
		PageSize:  page.PageSize,
		Total:     page.Total,
	}, nil
}

func TransformProductsToReplyForMP(products []*product2.Product) []types.Product {
	productsReply := []types.Product{}
	for _, product := range products {
		productReply := TransformProductToReplyForMP(product)
		productsReply = append(productsReply, *productReply)

	}
	return productsReply
}
