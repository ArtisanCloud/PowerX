package productstatistics

import (
	"PowerX/internal/model/crm/product"
	"PowerX/internal/types/errorx"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ConfigProductStatisticsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewConfigProductStatisticsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ConfigProductStatisticsLogic {
	return &ConfigProductStatisticsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ConfigProductStatisticsLogic) ConfigProductStatistics(req *types.ConfigProductStatisticsRequest) (resp *types.ConfigProductStatisticsReply, err error) {

	staticstics := TransformRequestToProductStaticstics(req.ProductStatistics)

	staticstics, err = l.svcCtx.PowerX.ProductStatistics.UpsertProductStatistics(l.ctx, staticstics)
	if err != nil {
		return nil, errorx.WithCause(errorx.ErrBadRequest, err.Error())
	}

	return &types.ConfigProductStatisticsReply{
		Result: true,
	}, nil
}

func TransformRequestToProductStaticstics(statistics *types.ProductStatistics) *product.ProductStatistics {

	return &product.ProductStatistics{
		ProductId:             statistics.ProductId,
		BaseSoldAmount:        statistics.BaseSoldAmount,
		BaseInventoryQuantity: statistics.BaseInventoryQuantity,
		BaseViewCount:         statistics.BaseViewCount,
	}
}
