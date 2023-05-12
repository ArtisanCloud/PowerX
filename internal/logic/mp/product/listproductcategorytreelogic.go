package product

import (
	product2 "PowerX/internal/logic/admin/product"
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

func (l *ListProductCategoryTreeLogic) ListProductCategoryTree(req *types.ListProductCategoryTreeRequest) (resp *types.ListProductCategoryTreeReply, err error) {
	option := product.FindProductCategoryOption{
		Names:   req.Names,
		OrderBy: req.OrderBy,
	}

	// 获取模型类型的列表
	productCategoryTree := l.svcCtx.PowerX.ProductCategory.ListProductCategoryTree(l.ctx, &option, 0)

	// 转化返回类型的列表
	productCategoryReplyList := product2.TransformProductCategoriesToProductCategoriesReply(productCategoryTree)

	return &types.ListProductCategoryTreeReply{
		ProductCategories: productCategoryReplyList,
	}, nil
}
