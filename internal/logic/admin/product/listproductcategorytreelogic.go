package product

import (
	"PowerX/internal/model/product"
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

func (l *ListProductCategoryTreeLogic) ListProductCategoryTree(req *types.GetProductCategoryTreeRequest) (resp *types.GetProductCategoryTreeReply, err error) {
	option := product.FindProductCategoryOption{
		Names:   req.Names,
		OrderBy: req.OrderBy,
	}

	// 获取模型类型的列表
	productCategoryTree := l.svcCtx.PowerX.ProductCategory.GetProductCategoryTree(l.ctx, &option, 0)

	// 转化返回类型的列表
	productCategoryReplyList := l.convertModelToTypeReply(productCategoryTree)

	return &types.GetProductCategoryTreeReply{
		ProductCategories: productCategoryReplyList,
	}, nil

	return
}

func (l *ListProductCategoryTreeLogic) convertModelToTypeReply(productCategoryList []*product.ProductCategory) []types.ProductCategory {
	var productCategoryReplyList []types.ProductCategory
	for _, category := range productCategoryList {
		node := types.ProductCategory{
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
			node.Children = l.convertModelToTypeReply(category.Children)

		}

		productCategoryReplyList = append(productCategoryReplyList, node)
	}

	return productCategoryReplyList
}
