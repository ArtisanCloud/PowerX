package sku

import (
	"PowerX/internal/types/errorx"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeleteSKULogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeleteSKULogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteSKULogic {
	return &DeleteSKULogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DeleteSKULogic) DeleteSKU(req *types.DeleteSKURequest) (resp *types.DeleteSKUReply, err error) {
	sku, err := l.svcCtx.PowerX.SKU.GetSKU(l.ctx, req.SKUId)
	if err != nil {
		return nil, errorx.ErrNotFoundObject
	}

	err = l.svcCtx.PowerX.SKU.DeleteSKU(l.ctx, sku.Id)
	if err != nil {
		return nil, err
	}

	return &types.DeleteSKUReply{
		SKUId: sku.Id,
	}, nil
}
