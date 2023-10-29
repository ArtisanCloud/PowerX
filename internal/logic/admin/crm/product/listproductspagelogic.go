package product

import (
	"PowerX/internal/model/crm/product"
	"PowerX/internal/svc"
	"PowerX/internal/types"
	productUC "PowerX/internal/uc/powerx/crm/product"
	"PowerX/pkg/datetime/carbonx"
	"context"
	"github.com/golang-module/carbon/v2"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListProductsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListProductsPageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListProductsLogic {
	return &ListProductsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListProductsLogic) ListProductsPage(req *types.ListProductsPageRequest) (resp *types.ListProductsPageReply, err error) {

	startAt := carbon.ParseByFormat(req.SalesStartAt, carbonx.DateFormat)
	endAt := carbon.ParseByFormat(req.SalesEndAt, carbonx.DateFormat)
	if !startAt.IsZero() && endAt.IsZero() {
		endAt = startAt.AddDays(30)
	} else if startAt.IsZero() && !endAt.IsZero() {
		startAt = endAt.AddDays(-30)
	}

	// 去掉代币的产品
	notInTypeId := l.svcCtx.PowerX.DataDictionary.GetCachedDDId(l.ctx, product.TypeProductType, product.ProductTypeToken)

	page, err := l.svcCtx.PowerX.Product.FindManyProducts(l.ctx, &productUC.FindManyProductsOption{
		StartAt:     startAt.ToStdTime(),
		EndAt:       endAt.ToStdTime(),
		LikeName:    req.LikeName,
		NotInTypes:  []int{notInTypeId},
		Types:       req.ProductTypeIds,
		CategoryIds: req.ProductCategoryIds,
		PageEmbedOption: types.PageEmbedOption{
			PageIndex: req.PageIndex,
			PageSize:  req.PageSize,
		},
	})
	if err != nil {
		return nil, err
	}

	// list
	list := TransformProductsToReply(page.List)
	return &types.ListProductsPageReply{
		List:      list,
		PageIndex: page.PageIndex,
		PageSize:  page.PageSize,
		Total:     page.Total,
	}, nil

}

func TransformProductsToReply(products []*product.Product) []types.Product {
	productsReply := []types.Product{}
	for _, product := range products {
		productReply := TransformProductToReply(product)
		productsReply = append(productsReply, *productReply)

	}
	return productsReply
}
