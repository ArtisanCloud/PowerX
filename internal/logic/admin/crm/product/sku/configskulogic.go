package sku

import (
	"PowerX/internal/model/crm/product"
	"PowerX/internal/types/errorx"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ConfigSKULogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewConfigSKULogic(ctx context.Context, svcCtx *svc.ServiceContext) *ConfigSKULogic {
	return &ConfigSKULogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ConfigSKULogic) ConfigSKU(req *types.ConfigSKURequest) (resp *types.ConfigSKUReply, err error) {
	skus := TransformRequestToSKUs(req.SKUs)
	//fmt.Dump(skus)
	err = l.svcCtx.PowerX.SKU.ConfigSKU(l.ctx, skus)
	if err != nil {
		return nil, errorx.WithCause(errorx.ErrBadRequest, err.Error())
	}

	return &types.ConfigSKUReply{
		Result: true,
	}, nil
}

func TransformRequestToSKUs(skusRequest []types.SKU) []*product.SKU {
	skus := []*product.SKU{}
	for _, skuRequest := range skusRequest {
		skus = append(skus, TransformSKURequestToSKU(skuRequest))
	}

	return skus
}
