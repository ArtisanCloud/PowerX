package product

import (
	product3 "PowerX/internal/uc/powerx/product"
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

func (l *ListProductCategoriesLogic) ListProductCategories(req *types.ListProductCategoriesRequest) (resp *types.ListProductCategoriesReply, err error) {
	option := product3.FindProductCategoryOption{
		CategoryPId: req.CategoryPId,
		Limit:       req.Limit,
	}

	// 获取模型类型的列表
	productCategoryTree := l.svcCtx.PowerX.ProductCategory.FindProductCategoriesByParentId(l.ctx, &option)

	// 转化返回类型的列表
	productCategoryReplyList := TransformProductCategoriesToProductCategoriesReplyToMP(productCategoryTree)

	return &types.ListProductCategoriesReply{
		ProductCategories: productCategoryReplyList,
	}, nil
}
