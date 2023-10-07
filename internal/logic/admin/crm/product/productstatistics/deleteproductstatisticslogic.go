package productstatistics

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeleteProductStatisticsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeleteProductStatisticsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteProductStatisticsLogic {
	return &DeleteProductStatisticsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DeleteProductStatisticsLogic) DeleteProductStatistics(req *types.DeleteProductStatisticsRequest) (resp *types.DeleteProductStatisticsReply, err error) {
	// todo: add your logic here and delete this line

	return
}
