package category

import (
	"PowerX/internal/logic/admin/mediaresource"
	infoorganizatoin "PowerX/internal/model/infoorganization"
	"PowerX/internal/svc"
	"PowerX/internal/types"
	"PowerX/internal/uc/powerx/crm/infoorganization"
	"context"
	"github.com/zeromicro/go-zero/core/logx"
)

type ListCategoryTreeLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListCategoryTreeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListCategoryTreeLogic {
	return &ListCategoryTreeLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListCategoryTreeLogic) ListCategoryTree(req *types.ListCategoryTreeRequest) (resp *types.ListCategoryTreeReply, err error) {
	option := infoorganization.FindCategoryOption{
		Names:   req.Names,
		OrderBy: req.OrderBy,
	}

	// 获取模型类型的列表
	productCategoryTree := l.svcCtx.PowerX.Category.ListCategoryTree(l.ctx, &option, 0)

	// 转化返回类型的列表
	productCategoryReplyList := TransformProductCategoriesToReply(productCategoryTree)

	return &types.ListCategoryTreeReply{
		ProductCategories: productCategoryReplyList,
	}, nil

}

func TransformProductCategoriesToReply(productCategoryList []*infoorganizatoin.Category) []*types.Category {
	uniqueIds := make(map[int64]bool)
	var productCategoryReplyList []*types.Category
	for _, category := range productCategoryList {
		if !uniqueIds[category.Id] {
			node := TransformCategoryToReply(category)
			if len(category.Children) > 0 {
				node.Children = TransformProductCategoriesToReply(category.Children)
			}

			productCategoryReplyList = append(productCategoryReplyList, node)
			uniqueIds[category.Id] = true

		}
	}

	return productCategoryReplyList
}

func TransformCategoryToReply(category *infoorganizatoin.Category) *types.Category {
	if category == nil {
		return nil
	}
	return &types.Category{
		Id:           category.Id,
		PId:          category.PId,
		Name:         category.Name,
		Sort:         category.Sort,
		ViceName:     category.ViceName,
		Description:  category.Description,
		CreatedAt:    category.CreatedAt.String(),
		CoverImageId: category.CoverImageId,
		CoverImage:   mediaresource.TransformMediaResourceToReply(category.CoverImage),
		ImageAbleInfo: types.ImageAbleInfo{
			Icon:            category.Icon,
			BackgroundColor: category.BackgroundColor,
		},
		Children: nil,
	}
}
