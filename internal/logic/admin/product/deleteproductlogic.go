package product

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeleteProductLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeleteProductLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteProductLogic {
	return &DeleteProductLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DeleteProductLogic) DeleteProduct(req *types.DeleteProductRequest) (resp *types.DeleteProductReply, err error) {

	err = l.svcCtx.PowerX.Product.DeleteProduct(l.ctx, req.ProductId)
	if err != nil {
		return nil, err
	}

	return &types.DeleteProductReply{
		ProductId: req.ProductId,
	}, nil
}
