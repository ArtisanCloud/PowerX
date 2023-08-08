package category

import (
	"PowerX/internal/model/media"
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
	productCategoryReplyList := TransformProductCategoriesToReply(productCategoryTree)

	return &types.ListProductCategoryTreeReply{
		ProductCategories: productCategoryReplyList,
	}, nil

}

func TransformProductCategoriesToReply(productCategoryList []*product.ProductCategory) []*types.ProductCategory {
	uniqueIds := make(map[int64]bool)
	var productCategoryReplyList []*types.ProductCategory
	for _, category := range productCategoryList {
		if !uniqueIds[category.Id] {
			node := TransformProductCategoryToReply(category)
			if len(category.Children) > 0 {
				node.Children = TransformProductCategoriesToReply(category.Children)
			}

			productCategoryReplyList = append(productCategoryReplyList, node)
			uniqueIds[category.Id] = true

		}
	}

	return productCategoryReplyList
}

func TransformProductCategoryToReply(category *product.ProductCategory) *types.ProductCategory {
	if category == nil {
		return nil
	}
	return &types.ProductCategory{
		Id:           category.Id,
		PId:          category.PId,
		Name:         category.Name,
		Sort:         category.Sort,
		ViceName:     category.ViceName,
		Description:  category.Description,
		CreatedAt:    category.CreatedAt.String(),
		CoverImageId: category.CoverImageId,
		CoverImage:   TransformCategoryImageToReply(category.CoverImage),
		ImageAbleInfo: types.ImageAbleInfo{
			Icon:            category.Icon,
			BackgroundColor: category.BackgroundColor,
		},
		Children: nil,
	}
}

func TransformCategoryImageToReply(resource *media.MediaResource) *types.MediaResource {
	if resource == nil {
		return nil
	}
	return &types.MediaResource{
		Id:            resource.Id,
		CustomerId:    resource.CustomerId,
		BucketName:    resource.BucketName,
		Filename:      resource.Filename,
		Size:          resource.Size,
		IsLocalStored: resource.IsLocalStored,
		Url:           resource.Url,
		ContentType:   resource.ContentType,
		ResourceType:  resource.ResourceType,
	}
}
