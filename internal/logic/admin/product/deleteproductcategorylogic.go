package product

import (
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

	err = l.svcCtx.PowerX.ProductCategory.DeleteProductCategory(l.ctx, req.ProductCategoryId)
	if err != nil {
		panic(err)
		return
	}

	return &types.DeleteProductCategoryReply{
		ProductCategoryId: req.ProductCategoryId,
	}, nil
}
