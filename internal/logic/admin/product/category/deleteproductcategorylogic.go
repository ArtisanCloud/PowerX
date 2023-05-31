package category

import (
	"PowerX/internal/types/errorx"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeleteProductCategoryLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeleteProductCategoryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteProductCategoryLogic {
	return &DeleteProductCategoryLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DeleteProductCategoryLogic) DeleteProductCategory(req *types.DeleteProductCategoryRequest) (resp *types.DeleteProductCategoryReply, err error) {

	productCategory, err := l.svcCtx.PowerX.ProductCategory.GetProductCategory(l.ctx, req.Id)
	if err != nil {
		return nil, errorx.ErrNotFoundObject
	}
	if len(productCategory.Children) > 0 {
		return nil, errorx.WithCause(errorx.ErrBadRequest, "该品类包含子子品类，请处理子品类")
	}

	err = l.svcCtx.PowerX.ProductCategory.DeleteProductCategory(l.ctx, req.Id)
	if err != nil {
		return nil, err
	}

	return &types.DeleteProductCategoryReply{
		Id: req.Id,
	}, nil
}
