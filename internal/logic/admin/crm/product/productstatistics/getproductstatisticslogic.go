package productstatistics

import (
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
	// todo: add your logic here and delete this line

	return
}
