package productstatistics

import (
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
	// todo: add your logic here and delete this line

	return
}
