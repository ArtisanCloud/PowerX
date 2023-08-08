package product

import (
	"PowerX/internal/svc"
	"PowerX/internal/types"
	"PowerX/internal/types/errorx"
	"context"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetProductLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetProductLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetProductLogic {
	return &GetProductLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetProductLogic) GetProduct(req *types.GetProductRequest) (resp *types.GetProductReply, err error) {
	mdlProduct, err := l.svcCtx.PowerX.Product.GetProduct(l.ctx, req.ProductId)

	if err != nil {
		return nil, errorx.ErrNotFoundObject
	}

	return &types.GetProductReply{
		Product: TransformProductToReply(mdlProduct),
	}, nil

}
