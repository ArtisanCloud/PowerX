package productstatistics

import (
	product2 "PowerX/internal/model/crm/product"
	"PowerX/internal/types/errorx"
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
	if err != nil {
		return nil, errorx.WithCause(err, err.Error())
	}

	return &types.GetProductStatisticsReply{
		ProductStatistics: TransformProductStatisticsToReplyForMP(statistics),
	}, nil
}

func TransformProductStatisticsToReplyForMP(specific *product2.ProductStatistics) (specificReply *types.ProductStatistics) {
	if specific == nil {
		return nil
	}

	return &types.ProductStatistics{
		Id:                specific.Id,
		ProductId:         specific.ProductId,
		SoldAmount:        specific.BaseSoldAmount + specific.SoldAmount,
		InventoryQuantity: specific.BaseInventoryQuantity + specific.InventoryQuantity,
		ViewCount:         specific.BaseViewCount + specific.ViewCount,
	}
}
