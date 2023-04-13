package product

import (
	"PowerX/internal/model/product"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListProductCategoriesLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListProductCategoriesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListProductCategoriesLogic {
	return &ListProductCategoriesLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListProductCategoriesLogic) ListProductCategories(req *types.GetProductCategoryListRequest) (resp *types.GetProductCategoryListReply, err error) {

	option := product.FindProductCategoryOption{
		PageIndex: req.PageIndex,
		PageSize:  req.PageSize,
	}

	productCategoryList := l.svcCtx.PowerX.ProductCategory.FindManyProductCategories(l.ctx, &option)

	var list []types.ProductCategory
	for _, category := range productCategoryList.List {
		list = append(list, types.ProductCategory{
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
		})
	}

	return &types.GetProductCategoryListReply{
		List:      list,
		PageIndex: productCategoryList.PageIndex,
		PageSize:  productCategoryList.PageSize,
		Total:     productCategoryList.Total,
	}, nil

}
