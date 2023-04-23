package product

import (
	"PowerX/internal/model"
	"PowerX/internal/svc"
	"PowerX/internal/types"
	"PowerX/internal/uc/powerx"
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

	mdlProduct := TransformProductRequestToProduct(&(req.Product))
	mdlProduct.Id = req.ProductId

	// 处理销售渠道
	if len(req.SalesChannelsItemIds) > 0 {
		salesChannelsItems, err := l.svcCtx.PowerX.DataDictionary.FindAllDictionaryItems(l.ctx, &powerx.FindManyDataDictItemOption{
			Ids: req.SalesChannelsItemIds,
		})
		if err != nil {
			return nil, err
		}
		mdlProduct.PivotSalesChannels, err = (&model.PivotDataDictionaryToObject{}).MakeMorphPivotsFromObjectToDDs(mdlProduct, salesChannelsItems)

	}

	// 处理推广渠道
	if len(req.PromoteChannelsItemIds) > 0 {
		promoteChannelsItems, err := l.svcCtx.PowerX.DataDictionary.FindAllDictionaryItems(l.ctx, &powerx.FindManyDataDictItemOption{
			Ids: req.PromoteChannelsItemIds,
		})
		if err != nil {
			return nil, err
		}
		mdlProduct.PivotPromoteChannels, err = (&model.PivotDataDictionaryToObject{}).MakeMorphPivotsFromObjectToDDs(mdlProduct, promoteChannelsItems)
	}

	// 更新产品对象
	mdlProduct, err = l.svcCtx.PowerX.Product.UpsertProduct(l.ctx, mdlProduct)
	if err != nil {
		return nil, err
	}

	return &types.PutProductReply{
		Product: TransformProductToProductReply(mdlProduct),
	}, nil

}
