package product

import (
	"PowerX/internal/svc"
	"PowerX/internal/types"
	"context"

	"github.com/zeromicro/go-zero/core/logx"
)

type PutProductLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPutProductLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PutProductLogic {
	return &PutProductLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PutProductLogic) PutProduct(req *types.PutProductRequest) (resp *types.PutProductReply, err error) {

	newModel := TransformProductRequestToProduct(&(req.Product))

	l.svcCtx.PowerX.Product.PatchProduct(l.ctx, req.ProductId, newModel)

	return &types.PutProductReply{
		Product: TransformProductToProductReply(newModel),
	}, nil

}
