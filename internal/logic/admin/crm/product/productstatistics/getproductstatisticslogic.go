package productstatistics

import (
	product2 "PowerX/internal/model/crm/product"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetProductStatisticsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetProductStatisticsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetProductStatisticsLogic {
	return &GetProductStatisticsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetProductStatisticsLogic) GetProductStatistics(req *types.GetProductStatisticsRequest) (resp *types.GetProductStatisticsReply, err error) {

	statistics, err := l.svcCtx.PowerX.ProductStatistics.GetProductStatisticsByProductId(l.ctx, req.ProductId)
	return &types.GetProductStatisticsReply{
		ProductStatistics: TransformProductStatisticsToReply(statistics),
	}, nil
}

func TransformProductStatisticsToReply(specific *product2.ProductStatistics) (specificReply *types.ProductStatistics) {
	if specific == nil {
		return nil
	}

	return &types.ProductStatistics{
		Id:                    specific.Id,
		ProductId:             specific.ProductId,
		SoldAmount:            specific.SoldAmount,
		InventoryQuantity:     specific.InventoryQuantity,
		ViewCount:             specific.ViewCount,
		BaseSoldAmount:        specific.BaseSoldAmount,
		BaseInventoryQuantity: specific.BaseInventoryQuantity,
		BaseViewCount:         specific.BaseViewCount,
	}
}
