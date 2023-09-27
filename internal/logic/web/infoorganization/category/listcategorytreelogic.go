package category

import (
	"PowerX/internal/logic/admin/infoorganization/category"
	"PowerX/internal/uc/powerx/crm/infoorganization"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

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
	productCategoryReplyList := category.TransformProductCategoriesToReply(productCategoryTree)

	return &types.ListCategoryTreeReply{
		ProductCategories: productCategoryReplyList,
	}, nil
}
