package sku

import (
	product2 "PowerX/internal/model/crm/product"
	"context"
	"github.com/ArtisanCloud/PowerLibs/v3/object"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateSKULogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateSKULogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateSKULogic {
	return &CreateSKULogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateSKULogic) CreateSKU(req *types.CreateSKURequest) (resp *types.CreateSKUReply, err error) {
	// todo: add your logic here and delete this line

	return
}

func TransformSKURequestToSKU(skuRequest types.SKU) *product2.SKU {

	if skuRequest.ProductId <= 0 || skuRequest.SkuNo == "" {
		return nil
	}
	sku := &product2.SKU{
		ProductId: skuRequest.ProductId,
		SkuNo:     skuRequest.SkuNo,
		Inventory: skuRequest.Inventory,
		UniqueID:  object.NewNullString(skuRequest.UniqueId, true),
	}

	if skuRequest.Id > 0 {
		sku.Id = skuRequest.Id
	}
	return sku
}
