package product

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type DisableProductLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDisableProductLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DisableProductLogic {
	return &DisableProductLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DisableProductLogic) DisableProduct(req *types.DisableProductRequest) (resp *types.DisableProductReply, err error) {
	p := &map[string]interface{}{
		"is_activated": false,
	}
	//fmt.Dump(p)
	l.svcCtx.PowerX.Product.PatchProduct(l.ctx, req.ProductId, p)

	return &types.DisableProductReply{
		ProductId: req.ProductId,
	}, nil
}
