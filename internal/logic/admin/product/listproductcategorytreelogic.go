package product

import (
	"PowerX/internal/model/product"
	product2 "PowerX/internal/uc/powerx/product"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListProductCategoryTreeLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListProductCategoryTreeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListProductCategoryTreeLogic {
	return &ListProductCategoryTreeLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListProductCategoryTreeLogic) ListProductCategoryTree(req *types.ListProductCategoryTreeRequest) (resp *types.ListProductCategoryTreeReply, err error) {
	option := product2.FindProductCategoryOption{
		Names:   req.Names,
		OrderBy: req.OrderBy,
	}

	// 获取模型类型的列表
	productCategoryTree := l.svcCtx.PowerX.ProductCategory.ListProductCategoryTree(l.ctx, &option, 0)

	// 转化返回类型的列表
	productCategoryReplyList := TransformProductCategoriesToProductCategoriesReply(productCategoryTree)

	return &types.ListProductCategoryTreeReply{
		ProductCategories: productCategoryReplyList,
	}, nil

}

func TransformProductCategoriesToProductCategoriesReply(productCategoryList []*product.ProductCategory) []*types.ProductCategory {
	var productCategoryReplyList []*types.ProductCategory
	for _, category := range productCategoryList {
		node := &types.ProductCategory{
			Id:          category.Id,
			PId:         category.PId,
			Name:        category.Name,
			Sort:        category.Sort,
			ViceName:    category.ViceName,
			Description: category.Description,
			CreatedAt:   category.CreatedAt.String(),
			ImageAbleInfo: types.ImageAbleInfo{
				Icon:            category.Icon,
				BackgroundColor: category.BackgroundColor,
				ImageURL:        category.ImageURL,
			},
			Children: nil,
		}
		if len(category.Children) > 0 {
			node.Children = TransformProductCategoriesToProductCategoriesReply(category.Children)

		}

		productCategoryReplyList = append(productCategoryReplyList, node)
	}

	return productCategoryReplyList
}
